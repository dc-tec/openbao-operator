package main

import "time"

const (
	metricReconcileP95      = "reconcile_p95_seconds"
	metricBackupLastMax     = "backup_last_duration_seconds_max"
	metricRestoreP95        = "restore_p95_seconds"
	metricUpgradeP95        = "upgrade_p95_seconds"
	metricUpgradePodP95     = "upgrade_pod_p95_seconds"
	metricWorkqueueRetries  = "workqueue_retries_delta"
	metricReconcileErrRatio = "reconcile_error_ratio"
)

var metricKeys = []string{
	metricReconcileP95,
	metricBackupLastMax,
	metricRestoreP95,
	metricUpgradeP95,
	metricUpgradePodP95,
	metricWorkqueueRetries,
	metricReconcileErrRatio,
}

var p95MetricSet = map[string]struct{}{
	metricReconcileP95:  {},
	metricRestoreP95:    {},
	metricUpgradeP95:    {},
	metricUpgradePodP95: {},
}

const (
	metricPolicyUpperBound = "upper_bound"
	metricPolicyMustBeZero = "must_be_zero"
	metricPolicyIgnore     = "ignore"

	metricSeverityFail = "fail"
	metricSeverityWarn = "warn"

	metricMultiplierP95 = "p95"
	metricMultiplierMax = "max"
)

type scenarioSpec struct {
	Name           string                      `json:"name" yaml:"name"`
	Description    string                      `json:"description,omitempty" yaml:"description,omitempty"`
	LabelFilter    string                      `json:"labelFilter" yaml:"labelFilter"`
	MetricPolicies map[string]metricPolicySpec `json:"metricPolicies" yaml:"metricPolicies"`
}

type scenarioManifest struct {
	Version   string         `json:"version" yaml:"version"`
	Scenarios []scenarioSpec `json:"scenarios" yaml:"scenarios"`
}

type metricPolicySpec struct {
	Policy     string   `json:"policy" yaml:"policy"`
	Severity   string   `json:"severity,omitempty" yaml:"severity,omitempty"`
	Multiplier string   `json:"multiplier,omitempty" yaml:"multiplier,omitempty"`
	Floor      *float64 `json:"floor,omitempty" yaml:"floor,omitempty"`
	Threshold  *float64 `json:"threshold,omitempty" yaml:"threshold,omitempty"`
}

type runResult struct {
	Scenario      string             `json:"scenario"`
	LabelFilter   string             `json:"labelFilter"`
	Run           int                `json:"run"`
	Cluster       string             `json:"cluster"`
	StartedAt     time.Time          `json:"startedAt"`
	Duration      time.Duration      `json:"duration"`
	Metrics       map[string]float64 `json:"metrics"`
	BeforePresent bool               `json:"beforePresent"`
}

type scenarioBaseline struct {
	LabelFilter    string                      `json:"labelFilter"`
	MetricPolicies map[string]metricPolicySpec `json:"metricPolicies,omitempty"`
	Runs           []runResult                 `json:"runs"`
	MaxMetrics     map[string]float64          `json:"maxMetrics"`
}

type baselineDocument struct {
	Version      string                      `json:"version"`
	CapturedAt   time.Time                   `json:"capturedAt"`
	NodeImage    string                      `json:"nodeImage"`
	RunsPerCase  int                         `json:"runsPerCase"`
	Multipliers  multiplierConfig            `json:"multipliers"`
	Scenarios    map[string]scenarioBaseline `json:"scenarios"`
	MetricSchema []string                    `json:"metricSchema"`
}

type multiplierConfig struct {
	P95 float64 `json:"p95" yaml:"p95"`
	Max float64 `json:"max" yaml:"max"`
}

type thresholdDocument struct {
	Version      string                        `json:"version" yaml:"version"`
	GeneratedAt  time.Time                     `json:"generatedAt" yaml:"generatedAt"`
	NodeImage    string                        `json:"nodeImage" yaml:"nodeImage"`
	Multipliers  multiplierConfig              `json:"multipliers" yaml:"multipliers"`
	MetricSchema []string                      `json:"metricSchema" yaml:"metricSchema"`
	Scenarios    map[string]scenarioThresholds `json:"scenarios" yaml:"scenarios"`
}

type scenarioThresholds struct {
	LabelFilter    string                      `json:"labelFilter" yaml:"labelFilter"`
	MetricPolicies map[string]metricPolicySpec `json:"metricPolicies,omitempty" yaml:"metricPolicies,omitempty"`
	Metrics        map[string]float64          `json:"metrics" yaml:"metrics"`
}

type options struct {
	Mode            string
	Runs            int
	ScenarioNames   []string
	ScenarioPath    string
	NodeImage       string
	KindBin         string
	MakeBin         string
	BaselinePath    string
	ThresholdsPath  string
	ScenarioTimeout time.Duration
	ClusterTimeout  time.Duration
	CleanupTimeout  time.Duration
	KeepOnFailure   bool
	P95Multiplier   float64
	MaxMultiplier   float64
	OperatorNS      string
	MetricsService  string
	ServiceAccount  string
	BindingName     string
	ThresholdsInput string
	Verbose         bool
}

type metricsSnapshot struct {
	Counters   map[string]float64
	GaugeMax   map[string]float64
	Histograms map[string]map[float64]float64
}
