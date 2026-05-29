package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

func parseMetricsSnapshot(metricsText string) (metricsSnapshot, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(metricsText))
	if err != nil {
		return metricsSnapshot{}, fmt.Errorf("parse metrics text: %w", err)
	}

	s := metricsSnapshot{
		Counters:   make(map[string]float64),
		GaugeMax:   make(map[string]float64),
		Histograms: make(map[string]map[float64]float64),
	}

	for name, family := range families {
		switch family.GetType() {
		case dto.MetricType_COUNTER:
			var total float64
			for _, m := range family.GetMetric() {
				total += m.GetCounter().GetValue()
			}
			s.Counters[name] = total
		case dto.MetricType_GAUGE:
			maxVal := math.Inf(-1)
			for _, m := range family.GetMetric() {
				val := m.GetGauge().GetValue()
				if val > maxVal {
					maxVal = val
				}
			}
			if maxVal > math.Inf(-1) {
				s.GaugeMax[name] = maxVal
			}
		case dto.MetricType_HISTOGRAM:
			if _, ok := s.Histograms[name]; !ok {
				s.Histograms[name] = make(map[float64]float64)
			}
			for _, m := range family.GetMetric() {
				for _, b := range m.GetHistogram().GetBucket() {
					s.Histograms[name][b.GetUpperBound()] += float64(b.GetCumulativeCount())
				}
			}
		}
	}

	return s, nil
}

func counterDelta(before, after metricsSnapshot, metric string) float64 {
	delta := after.Counters[metric] - before.Counters[metric]
	if delta < 0 {
		return 0
	}
	return delta
}

func histogramDelta(before, after metricsSnapshot, metric string) map[float64]float64 {
	out := make(map[float64]float64)
	afterBuckets := after.Histograms[metric]
	beforeBuckets := before.Histograms[metric]
	for le, afterVal := range afterBuckets {
		delta := afterVal - beforeBuckets[le]
		if delta < 0 {
			delta = 0
		}
		out[le] = delta
	}
	return out
}

func histogramP95UpperBound(cumulative map[float64]float64) float64 {
	const quantile = 0.95

	if len(cumulative) == 0 {
		return 0
	}

	finiteBounds := make([]float64, 0, len(cumulative))
	maxFinite := 0.0
	hasFinite := false
	for le := range cumulative {
		if math.IsInf(le, 1) {
			continue
		}
		finiteBounds = append(finiteBounds, le)
		if !hasFinite || le > maxFinite {
			hasFinite = true
			maxFinite = le
		}
	}
	if !hasFinite {
		return 0
	}
	sort.Float64s(finiteBounds)

	total := cumulative[math.Inf(1)]
	if total <= 0 {
		total = cumulative[maxFinite]
	}
	if total <= 0 {
		return 0
	}

	target := total * quantile
	for _, le := range finiteBounds {
		if cumulative[le] >= target {
			return le
		}
	}

	return maxFinite
}

func computeDiagnosticMeasurements(before, after metricsSnapshot) map[string]float64 {
	metrics := make(map[string]float64, len(diagnosticMetricKeys))

	addHistogramDiagnostic(metrics, before, after, metricReconcileDurationBucketP95, "openbao_reconcile_duration_seconds")
	if value, ok := after.GaugeMax["openbao_backup_last_duration_seconds"]; ok {
		metrics[metricBackupLastDurationSeconds] = value
	}
	addHistogramDiagnostic(metrics, before, after, metricRestoreDurationBucketP95, "openbao_restore_duration_seconds")
	addHistogramDiagnostic(metrics, before, after, metricUpgradeDurationBucketP95, "openbao_upgrade_duration_seconds")
	addHistogramDiagnostic(
		metrics,
		before,
		after,
		metricUpgradePodDurationBucketP95,
		"openbao_upgrade_pod_duration_seconds",
	)
	addCounterDiagnostic(metrics, before, after, metricWorkqueueRetriesDelta, "workqueue_retries_total")

	if hasCounterMetric(before, after, "openbao_reconcile_errors_total") ||
		hasCounterMetric(before, after, "controller_runtime_reconcile_total") {
		errDelta := counterDelta(before, after, "openbao_reconcile_errors_total")
		reconcileDelta := counterDelta(before, after, "controller_runtime_reconcile_total")
		metrics[metricReconcileErrorRatio] = reconcileErrorRatio(errDelta, reconcileDelta)
	}
	addCounterDiagnostic(metrics, before, after, metricKubernetesWrites, "openbao_kube_client_requests_total")
	addCounterDiagnostic(metrics, before, after, metricOpenBaoAPIRequests, "openbao_client_requests_total")
	addCounterDiagnostic(metrics, before, after, metricOpenBaoAuthLogins, "openbao_client_auth_logins_total")
	addCounterDiagnostic(metrics, before, after, metricOpenBaoAuthLoginErrors, "openbao_client_auth_login_errors_total")
	addCounterDiagnostic(metrics, before, after, metricOpenBaoClientRetries, "openbao_client_retries_total")
	addCounterDiagnostic(metrics, before, after, metricOpenBaoAuthCacheHits, "openbao_client_auth_cache_hits_total")
	addCounterDiagnostic(metrics, before, after, metricOpenBaoAuthCacheMisses, "openbao_client_auth_cache_misses_total")

	return metrics
}

func addHistogramDiagnostic(
	metrics map[string]float64,
	before metricsSnapshot,
	after metricsSnapshot,
	measurement string,
	source string,
) {
	if !hasHistogramMetric(before, after, source) {
		return
	}
	metrics[measurement] = histogramP95UpperBound(histogramDelta(before, after, source))
}

func addCounterDiagnostic(
	metrics map[string]float64,
	before metricsSnapshot,
	after metricsSnapshot,
	measurement string,
	source string,
) {
	if !hasCounterMetric(before, after, source) {
		return
	}
	metrics[measurement] = counterDelta(before, after, source)
}

func hasCounterMetric(before, after metricsSnapshot, metric string) bool {
	_, beforeOK := before.Counters[metric]
	_, afterOK := after.Counters[metric]
	return beforeOK || afterOK
}

func hasHistogramMetric(before, after metricsSnapshot, metric string) bool {
	_, beforeOK := before.Histograms[metric]
	_, afterOK := after.Histograms[metric]
	return beforeOK || afterOK
}

func reconcileErrorRatio(errorDelta, reconcileDelta float64) float64 {
	if reconcileDelta <= 0 {
		if errorDelta > 0 {
			return 1
		}
		return 0
	}
	r := errorDelta / reconcileDelta
	if r < 0 {
		return 0
	}
	return r
}
