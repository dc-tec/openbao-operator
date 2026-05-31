package main

import (
	"math"
	"testing"
)

func TestHistogramQuantileUpperBound(t *testing.T) {
	buckets := map[float64]float64{
		0.5:         4,
		1:           8,
		2:           12,
		5:           18,
		10:          20,
		math.Inf(1): 20,
	}

	got := histogramP95UpperBound(buckets)
	if got != 10 {
		t.Fatalf("histogramP95UpperBound() = %v, want 10", got)
	}
}

func TestCounterDeltaClampsResets(t *testing.T) {
	before := metricsSnapshot{Counters: map[string]float64{"x": 20}}
	after := metricsSnapshot{Counters: map[string]float64{"x": 10}}

	got := counterDelta(before, after, "x")
	if got != 0 {
		t.Fatalf("counterDelta() = %v, want 0", got)
	}
}

func TestComputeDiagnosticMeasurementsOmitsMissingMetrics(t *testing.T) {
	before := emptySnapshot()
	after := emptySnapshot()

	got := computeDiagnosticMeasurements(before, after)
	if len(got) != 0 {
		t.Fatalf("diagnostics for empty snapshots = %+v, want empty", got)
	}

	before.Counters["openbao_client_requests_total"] = 7
	after.Counters["openbao_client_requests_total"] = 7
	got = computeDiagnosticMeasurements(before, after)
	if _, exists := got[metricOpenBaoAPIRequests]; !exists {
		t.Fatalf("%s should be present when source counter exists", metricOpenBaoAPIRequests)
	}
	if _, exists := got[metricOpenBaoAuthLogins]; exists {
		t.Fatalf("%s should be omitted when source counter is absent", metricOpenBaoAuthLogins)
	}
}

func TestComputeDiagnosticMeasurementsFromOpenBaoWorkloadTelemetry(t *testing.T) {
	metricsText := `
# HELP vault_core_handle_request Request handling latency.
# TYPE vault_core_handle_request summary
vault_core_handle_request{quantile="0.5"} 0.03
vault_core_handle_request_sum 0.12
vault_core_handle_request_count 3
# HELP openbao_core_handle_login_request Login request latency.
# TYPE openbao_core_handle_login_request summary
openbao_core_handle_login_request_sum 0.20
openbao_core_handle_login_request_count 2
# HELP vault_core_check_token Token check latency.
# TYPE vault_core_check_token summary
vault_core_check_token_sum 0.05
vault_core_check_token_count 5
# HELP openbao_core_in_flight_requests In-flight requests.
# TYPE openbao_core_in_flight_requests gauge
openbao_core_in_flight_requests 7
# HELP vault_token_creation Token creations.
# TYPE vault_token_creation counter
vault_token_creation 4
# HELP vault_audit_log_request_failure Audit request failures.
# TYPE vault_audit_log_request_failure counter
vault_audit_log_request_failure 1
# HELP vault_audit_log_response_failure Audit response failures.
# TYPE vault_audit_log_response_failure counter
vault_audit_log_response_failure 2
`
	after, err := parseMetricsSnapshot(metricsText)
	if err != nil {
		t.Fatalf("parse workload metrics: %v", err)
	}

	got := computeDiagnosticMeasurements(emptySnapshot(), after)
	assertFloat(t, got[metricOpenBaoWorkloadRequests], 3)
	assertFloat(t, got[metricOpenBaoWorkloadRequestAvg], 0.04)
	assertFloat(t, got[metricOpenBaoWorkloadLogins], 2)
	assertFloat(t, got[metricOpenBaoWorkloadLoginAvg], 0.10)
	assertFloat(t, got[metricOpenBaoWorkloadTokenChecks], 5)
	assertFloat(t, got[metricOpenBaoWorkloadTokenCheckAvg], 0.01)
	assertFloat(t, got[metricOpenBaoWorkloadInFlightMax], 7)
	assertFloat(t, got[metricOpenBaoWorkloadTokenCreates], 4)
	assertFloat(t, got[metricOpenBaoWorkloadAuditRequestFailures], 1)
	assertFloat(t, got[metricOpenBaoWorkloadAuditResponseFailures], 2)
}

func TestReconcileErrorRatioWhenDenominatorZero(t *testing.T) {
	if got := reconcileErrorRatio(0, 0); got != 0 {
		t.Fatalf("reconcileErrorRatio(0,0) = %v, want 0", got)
	}
	if got := reconcileErrorRatio(2, 0); got != 1 {
		t.Fatalf("reconcileErrorRatio(2,0) = %v, want 1", got)
	}
}

func assertFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("value mismatch: got=%v want=%v", got, want)
	}
}
