package main

import (
	"context"
	"time"
)

const (
	versionV2 = "v2"

	defaultScenarioPath = "hack/perf/v2/scenarios.yaml"
	defaultPolicyPath   = "hack/perf/v2/policies/weekly.yaml"
	defaultBaselineDir  = "hack/perf/v2/baselines"
	defaultArtifactDir  = "dist/perf"
	defaultEnvironment  = "kind-v1.34.3"

	executorE2EGinkgo = "e2e-ginkgo"
	executorNativeGo  = "native-go"
	executorScript    = "script"

	cleanupAlways    = "always"
	cleanupOnSuccess = "on_success"
	cleanupNever     = "never"

	sampleStatusPass             = "pass"
	sampleStatusScenarioError    = "scenario_error"
	sampleStatusMeasurementError = "measurement_error"

	measurementPolicyUpperBound    = "upper_bound"
	measurementPolicyLowerBound    = "lower_bound"
	measurementPolicyMustBeZero    = "must_be_zero"
	measurementPolicyInformational = "informational"

	measurementSeverityFail = "fail"
	measurementSeverityWarn = "warn"
	measurementSeverityInfo = "info"

	findingPerformanceFailure            = "performance_failure"
	findingPerformanceFailureConsecutive = "performance_failure_consecutive"

	measurementRolePrimary    = "primary"
	measurementRoleDiagnostic = "diagnostic"

	compareMedian      = "median"
	compareMax         = "max"
	compareUpperSample = "upper_sample"

	metricSampleTotalSeconds                   = "sample_total_seconds"
	metricScenarioRunSeconds                   = "scenario_run_seconds"
	metricReconcileDurationBucketP95           = "reconcile_duration_bucket_p95"
	metricBackupLastDurationSeconds            = "backup_last_duration_seconds"
	metricBackupTotalSeconds                   = "backup_total_seconds"
	metricBackupRequestToJobSeconds            = "backup_request_to_job_seconds"
	metricBackupJobDurationSeconds             = "backup_job_duration_seconds"
	metricRestoreDurationBucketP95             = "restore_duration_bucket_p95"
	metricRestoreTotalSeconds                  = "restore_total_seconds"
	metricRestoreValidationSeconds             = "restore_validation_seconds"
	metricRestoreJobDurationSeconds            = "restore_job_duration_seconds"
	metricUpgradeDurationBucketP95             = "upgrade_duration_bucket_p95"
	metricUpgradePodDurationBucketP95          = "upgrade_pod_duration_bucket_p95"
	metricWorkqueueRetriesDelta                = "workqueue_retries_delta"
	metricReconcileErrorRatio                  = "reconcile_error_ratio"
	metricKubernetesWrites                     = "kubernetes_writes"
	metricOpenBaoAPIRequests                   = "openbao_api_requests"
	metricOpenBaoAuthLogins                    = "openbao_auth_logins"
	metricOpenBaoAuthLoginErrors               = "openbao_auth_login_errors"
	metricOpenBaoAuthInlineRequests            = "openbao_auth_inline_requests"
	metricOpenBaoClientRetries                 = "openbao_api_retry_total"
	metricOpenBaoAuthCacheHits                 = "openbao_auth_cache_hits_total"
	metricOpenBaoAuthCacheMisses               = "openbao_auth_cache_misses_total"
	metricOpenBaoWorkloadRequests              = "openbao_workload_request_count"
	metricOpenBaoWorkloadRequestAvg            = "openbao_workload_request_avg_seconds"
	metricOpenBaoWorkloadLogins                = "openbao_workload_login_request_count"
	metricOpenBaoWorkloadLoginAvg              = "openbao_workload_login_request_avg_seconds"
	metricOpenBaoWorkloadTokenChecks           = "openbao_workload_token_check_count"
	metricOpenBaoWorkloadTokenCheckAvg         = "openbao_workload_token_check_avg_seconds"
	metricOpenBaoWorkloadInFlightMax           = "openbao_workload_in_flight_requests_max"
	metricOpenBaoWorkloadTokenCreates          = "openbao_workload_token_creation_count"
	metricOpenBaoWorkloadAuditRequestFailures  = "openbao_workload_audit_request_failures"
	metricOpenBaoWorkloadAuditResponseFailures = "openbao_workload_audit_response_failures"
	metricClusterAvailableSeconds              = "cluster_available_seconds"
	metricStatefulSetCreatedSeconds            = "statefulset_created_seconds"
	metricFirstPodReadySeconds                 = "first_pod_ready_seconds"
	metricAllPodsReadySeconds                  = "all_pods_ready_seconds"
	metricObservedKubernetesWrites             = "observed_kubernetes_writes"
	metricUpgradeTotalSeconds                  = "upgrade_total_seconds"
	metricUpgradeSessionStartSeconds           = "upgrade_session_start_seconds"
	metricUpgradePodReadySeconds               = "upgrade_pod_ready_seconds"
	metricUpgradeAvailabilityFailures          = "upgrade_availability_probe_failures"
	metricUpgradeKubernetesWrites              = "upgrade_kubernetes_writes"
	metricTenantChurnCompleteSeconds           = "tenant_churn_complete_seconds"
	metricTenantReadyP50Seconds                = "tenant_ready_p50_seconds"
	metricTenantReadyP95Seconds                = "tenant_ready_p95_seconds"
	metricTenantKubernetesWrites               = "tenant_kubernetes_writes"
	metricTenantCount                          = "tenant_count"
)

var diagnosticMetricKeys = []string{
	metricReconcileDurationBucketP95,
	metricBackupLastDurationSeconds,
	metricRestoreDurationBucketP95,
	metricUpgradeDurationBucketP95,
	metricUpgradePodDurationBucketP95,
	metricWorkqueueRetriesDelta,
	metricReconcileErrorRatio,
	metricKubernetesWrites,
	metricOpenBaoAPIRequests,
	metricOpenBaoAuthLogins,
	metricOpenBaoAuthLoginErrors,
	metricOpenBaoAuthInlineRequests,
	metricOpenBaoClientRetries,
	metricOpenBaoAuthCacheHits,
	metricOpenBaoAuthCacheMisses,
	metricOpenBaoWorkloadRequests,
	metricOpenBaoWorkloadRequestAvg,
	metricOpenBaoWorkloadLogins,
	metricOpenBaoWorkloadLoginAvg,
	metricOpenBaoWorkloadTokenChecks,
	metricOpenBaoWorkloadTokenCheckAvg,
	metricOpenBaoWorkloadInFlightMax,
	metricOpenBaoWorkloadTokenCreates,
	metricOpenBaoWorkloadAuditRequestFailures,
	metricOpenBaoWorkloadAuditResponseFailures,
}

type scenarioManifest struct {
	Version   string           `json:"version" yaml:"version"`
	Defaults  scenarioDefaults `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Scenarios []scenarioSpec   `json:"scenarios" yaml:"scenarios"`
}

type scenarioDefaults struct {
	Warmups       *int         `json:"warmups,omitempty" yaml:"warmups,omitempty"`
	Samples       *int         `json:"samples,omitempty" yaml:"samples,omitempty"`
	SampleTimeout yamlDuration `json:"sampleTimeout,omitempty" yaml:"sampleTimeout,omitempty"`
	Cleanup       string       `json:"cleanup,omitempty" yaml:"cleanup,omitempty"`
	ArtifactLevel string       `json:"artifactLevel,omitempty" yaml:"artifactLevel,omitempty"`
}

type scenarioSpec struct {
	Name            string                         `json:"name" yaml:"name"`
	Description     string                         `json:"description,omitempty" yaml:"description,omitempty"`
	Labels          []string                       `json:"labels,omitempty" yaml:"labels,omitempty"`
	Executor        string                         `json:"executor" yaml:"executor"`
	LabelFilter     string                         `json:"labelFilter,omitempty" yaml:"labelFilter,omitempty"`
	Command         []string                       `json:"command,omitempty" yaml:"command,omitempty"`
	Warmups         *int                           `json:"warmups,omitempty" yaml:"warmups,omitempty"`
	Samples         *int                           `json:"samples,omitempty" yaml:"samples,omitempty"`
	SampleTimeout   yamlDuration                   `json:"sampleTimeout,omitempty" yaml:"sampleTimeout,omitempty"`
	Cleanup         string                         `json:"cleanup,omitempty" yaml:"cleanup,omitempty"`
	ArtifactLevel   string                         `json:"artifactLevel,omitempty" yaml:"artifactLevel,omitempty"`
	Primary         []string                       `yaml:"primaryMeasurements,omitempty"`
	Diagnostic      []string                       `yaml:"diagnosticMeasurements,omitempty"`
	Phases          []phaseSpec                    `yaml:"phases,omitempty"`
	ExistingCluster existingClusterScenarioSupport `yaml:"existingCluster,omitempty"`
}

type phaseSpec struct {
	Name     string `json:"name" yaml:"name"`
	Required *bool  `json:"required,omitempty" yaml:"required,omitempty"`
}

type existingClusterScenarioSupport struct {
	Enabled     bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Destructive bool `json:"destructive,omitempty" yaml:"destructive,omitempty"`
}

type policyDocument struct {
	Version      string                       `json:"version" yaml:"version"`
	Defaults     measurementPolicy            `json:"defaults,omitempty" yaml:"defaults,omitempty"`
	Measurements map[string]measurementPolicy `json:"measurements" yaml:"measurements"`
}

type measurementPolicy struct {
	Role            string  `json:"role,omitempty" yaml:"role,omitempty"`
	Policy          string  `json:"policy,omitempty" yaml:"policy,omitempty"`
	Severity        string  `json:"severity,omitempty" yaml:"severity,omitempty"`
	Compare         string  `json:"compare,omitempty" yaml:"compare,omitempty"`
	AllowedRelative float64 `yaml:"allowedRelativeRegression,omitempty"`
	AllowedAbsolute float64 `yaml:"allowedAbsoluteRegressionSeconds,omitempty"`
	MinimumSamples  int     `yaml:"minimumSamples,omitempty"`
}

type sampleDocument struct {
	Version      string             `json:"version"`
	Scenario     string             `json:"scenario"`
	Sample       int                `json:"sample"`
	Warmup       bool               `json:"warmup"`
	Cluster      string             `json:"cluster,omitempty"`
	Environment  runEnvironment     `json:"environment"`
	StartedAt    time.Time          `json:"startedAt"`
	CompletedAt  time.Time          `json:"completedAt,omitempty"`
	Status       string             `json:"status"`
	Error        string             `json:"error,omitempty"`
	Phases       []phaseEvent       `json:"phases,omitempty"`
	Measurements map[string]float64 `json:"measurements"`
	Artifacts    map[string]string  `json:"artifacts,omitempty"`
}

type phaseEvent struct {
	Name   string    `json:"name"`
	At     time.Time `json:"at"`
	Source string    `json:"source"`
}

type runEnvironment struct {
	Runner            string `json:"runner,omitempty"`
	RunnerImage       string `json:"runnerImage,omitempty"`
	KindVersion       string `json:"kindVersion,omitempty"`
	NodeImage         string `json:"nodeImage,omitempty"`
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`
	GoVersion         string `json:"goVersion,omitempty"`
	Commit            string `json:"commit,omitempty"`
	Context           string `json:"context,omitempty"`
}

type baselineDocument struct {
	Version     string                        `json:"version"`
	Scenario    string                        `json:"scenario"`
	CapturedAt  time.Time                     `json:"capturedAt"`
	Commit      string                        `json:"commit,omitempty"`
	Environment runEnvironment                `json:"environment"`
	Samples     map[string][]float64          `json:"samples"`
	Summary     map[string]measurementSummary `json:"summary"`
}

type measurementSummary struct {
	Median      float64 `json:"median"`
	UpperSample float64 `json:"upperSample"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Count       int     `json:"count"`
}

type runSummaryDocument struct {
	Version     string                     `json:"version"`
	GeneratedAt time.Time                  `json:"generatedAt"`
	RunID       string                     `json:"runID,omitempty"`
	ArtifactDir string                     `json:"artifactDir"`
	BaselineDir string                     `json:"baselineDir,omitempty"`
	PolicyPath  string                     `json:"policyPath,omitempty"`
	PreviousRun string                     `json:"previousRun,omitempty"`
	Totals      summaryTotals              `json:"totals"`
	Scenarios   map[string]scenarioSummary `json:"scenarios"`
}

type summaryTotals struct {
	Scenarios int `json:"scenarios"`
	Samples   int `json:"samples"`
	Pass      int `json:"pass"`
	Warn      int `json:"warn"`
	Fail      int `json:"fail"`
}

type scenarioSummary struct {
	Status       string                        `json:"status"`
	Samples      int                           `json:"samples"`
	Warmups      int                           `json:"warmups"`
	Measurements map[string]measurementSummary `json:"measurements,omitempty"`
	Findings     []analysisFinding             `json:"findings,omitempty"`
}

type analysisFinding struct {
	Scenario       string  `json:"scenario"`
	Measurement    string  `json:"measurement,omitempty"`
	Severity       string  `json:"severity"`
	Classification string  `json:"classification"`
	Message        string  `json:"message"`
	Current        float64 `json:"current,omitempty"`
	Baseline       float64 `json:"baseline,omitempty"`
}

type options struct {
	Mode                   string
	ScenarioNames          []string
	ScenarioPath           string
	PolicyPath             string
	BaselineDir            string
	ArtifactDir            string
	PreviousSummaryPath    string
	SummaryOut             string
	ReportOut              string
	FailOnFailures         bool
	RunID                  string
	EnvironmentID          string
	NodeImage              string
	KindBin                string
	MakeBin                string
	ScenarioTimeout        time.Duration
	ClusterTimeout         time.Duration
	CleanupTimeout         time.Duration
	KeepOnFailure          bool
	ContinueOnSampleError  bool
	SamplesOverride        int
	WarmupsOverride        int
	ExistingClusterContext string
	Namespace              string
	NamespacePrefix        string
	SkipImageBuild         bool
	OperatorImage          string
	ConfigInitImage        string
	BackupExecutorImage    string
	UpgradeExecutorImage   string
	OpenBaoVersion         string
	OpenBaoImage           string
	UpgradeFromVersion     string
	UpgradeFromImage       string
	UpgradeToVersion       string
	UpgradeToImage         string
	APIServerCIDR          string
	StorageClass           string
	TenantChurnCount       int
	OperatorNS             string
	MetricsService         string
	ServiceAccount         string
	BindingName            string
}

type metricsSnapshot struct {
	Counters   map[string]float64
	GaugeMax   map[string]float64
	Histograms map[string]map[float64]float64
	Summaries  map[string]summarySnapshot
}

type summarySnapshot struct {
	Count float64
	Sum   float64
}

type yamlDuration struct {
	time.Duration
	set bool
}

type scenarioExecutionResult struct {
	Phases       []phaseEvent
	Measurements map[string]float64
	Namespace    string
	Artifacts    map[string]string
	Cleanup      func(context.Context)
}
