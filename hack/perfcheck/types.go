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

type scenarioSpec struct {
	Name        string `json:"name" yaml:"name"`
	LabelFilter string `json:"labelFilter" yaml:"labelFilter"`
}

var defaultScenarios = []scenarioSpec{
	{
		Name:        "lifecycle",
		LabelFilter: "lifecycle && critical && tenant && !slow && !openshift && !pentest",
	},
	{
		Name:        "backup-restore",
		LabelFilter: "dr && backup && restore && !failure-injection",
	},
	{
		Name:        "rolling-upgrade",
		LabelFilter: "upgrade && rolling && !failure && !bluegreen",
	},
}

var scenarioByName = map[string]scenarioSpec{
	"lifecycle":       defaultScenarios[0],
	"backup-restore":  defaultScenarios[1],
	"rolling-upgrade": defaultScenarios[2],
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
	LabelFilter string             `json:"labelFilter"`
	Runs        []runResult        `json:"runs"`
	MaxMetrics  map[string]float64 `json:"maxMetrics"`
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
	LabelFilter string             `json:"labelFilter" yaml:"labelFilter"`
	Metrics     map[string]float64 `json:"metrics" yaml:"metrics"`
}

type options struct {
	Mode            string
	Runs            int
	ScenarioNames   []string
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
