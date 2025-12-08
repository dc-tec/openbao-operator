package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	openBaoClusterWebhookLog = ctrl.Log.WithName("openbaocluster-webhook")

	configKeyPattern         = regexp.MustCompile(`^[a-z0-9_]+$`)
	selfInitRequestNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

const (
	maxConfigEntries        = 64
	maxConfigKeyLength      = 128
	maxConfigValueLength    = 4096
	maxSelfInitRequests     = 64
	maxSelfInitRequestPath  = 256
	configFieldPathRoot     = "spec"
	configFieldPathConfig   = "config"
	configFieldPathSelfInit = "selfInit"
	configFieldPathGateway  = "gateway"
	configFieldPathBackup   = "backup"
	// minBackupScheduleInterval is the minimum allowed interval between backups.
	minBackupScheduleInterval = 5 * time.Minute
	// warnBackupScheduleInterval is the interval below which we warn about frequent backups.
	warnBackupScheduleInterval = 10 * time.Minute
)

// cronParser is used to parse cron expressions for backup schedules.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// openBaoClusterValidator implements admission.CustomValidator for OpenBaoCluster.
type openBaoClusterValidator struct{}

// Ensure openBaoClusterValidator satisfies the CustomValidator interface.
var _ webhook.CustomValidator = &openBaoClusterValidator{}

// openBaoClusterDefaulter implements admission.CustomDefaulter for OpenBaoCluster.
// It is responsible for injecting default values that are independent of
// reconciliation logic, such as the finalizer used for cleanup.
type openBaoClusterDefaulter struct{}

// Ensure openBaoClusterDefaulter satisfies the CustomDefaulter interface.
var _ webhook.CustomDefaulter = &openBaoClusterDefaulter{}

// Default sets default values on OpenBaoCluster resources during admission.
// It injects the OpenBaoClusterFinalizer so that all delete operations go
// through the controller's finalizer-based cleanup path.
func (d *openBaoClusterDefaulter) Default(_ context.Context, obj runtime.Object) error {
	cluster, ok := obj.(*OpenBaoCluster)
	if !ok {
		return apierrors.NewBadRequest("expected OpenBaoCluster object for defaulting")
	}

	// During deletion, the controller must be able to remove the finalizer.
	// If the defaulter re-adds it on update, the OpenBaoCluster will get stuck
	// in a terminating state.
	if cluster.DeletionTimestamp != nil && !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	if !containsString(cluster.Finalizers, OpenBaoClusterFinalizer) {
		cluster.Finalizers = append(cluster.Finalizers, OpenBaoClusterFinalizer)
	}

	return nil
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}

	return false
}

// SetupWebhookWithManager registers the OpenBaoCluster validating webhook with the manager.
func (r *OpenBaoCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&OpenBaoCluster{}).
		WithValidator(&openBaoClusterValidator{}).
		WithDefaulter(&openBaoClusterDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-openbao-org-v1alpha1-openbaocluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=openbao.org,resources=openbaoclusters,verbs=create;update,versions=v1alpha1,name=mopenbaocluster.kb.io,admissionReviewVersions=v1

// +kubebuilder:webhook:path=/validate-openbao-org-v1alpha1-openbaocluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=openbao.org,resources=openbaoclusters,verbs=create;update,versions=v1alpha1,name=vopenbaocluster.kb.io,admissionReviewVersions=v1

// ValidateCreate validates OpenBaoCluster resources on create.
func (v *openBaoClusterValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cluster, ok := obj.(*OpenBaoCluster)
	if !ok {
		return nil, apierrors.NewBadRequest("expected OpenBaoCluster object for validation")
	}

	openBaoClusterWebhookLog.Info("validating create", "name", cluster.Name, "namespace", cluster.Namespace)

	var allErrs field.ErrorList
	var warnings admission.Warnings

	allErrs = append(allErrs, validateConfig(cluster)...)
	allErrs = append(allErrs, validateInitContainer(cluster)...)
	allErrs = append(allErrs, validateSelfInit(cluster)...)
	allErrs = append(allErrs, validateGateway(cluster)...)
	allErrs = append(allErrs, validateAudit(cluster)...)
	allErrs = append(allErrs, validatePlugins(cluster)...)
	allErrs = append(allErrs, validateTelemetry(cluster)...)
	allErrs = append(allErrs, validateTLS(cluster)...)
	networkErrs, networkWarnings := validateNetwork(cluster)
	allErrs = append(allErrs, networkErrs...)
	warnings = append(warnings, networkWarnings...)
	allErrs = append(allErrs, validateProfile(cluster)...)

	backupErrs, backupWarnings := validateBackup(cluster)
	allErrs = append(allErrs, backupErrs...)
	warnings = append(warnings, backupWarnings...)

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			GroupVersion.WithKind("OpenBaoCluster").GroupKind(),
			cluster.Name,
			allErrs,
		)
	}

	return warnings, nil
}

// ValidateUpdate validates OpenBaoCluster resources on update.
func (v *openBaoClusterValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	cluster, ok := newObj.(*OpenBaoCluster)
	if !ok {
		return nil, apierrors.NewBadRequest("expected OpenBaoCluster object for validation")
	}

	openBaoClusterWebhookLog.Info("validating update", "name", cluster.Name, "namespace", cluster.Namespace)

	var allErrs field.ErrorList
	var warnings admission.Warnings

	allErrs = append(allErrs, validateConfig(cluster)...)
	allErrs = append(allErrs, validateInitContainer(cluster)...)
	allErrs = append(allErrs, validateSelfInit(cluster)...)
	allErrs = append(allErrs, validateGateway(cluster)...)
	allErrs = append(allErrs, validateAudit(cluster)...)
	allErrs = append(allErrs, validatePlugins(cluster)...)
	allErrs = append(allErrs, validateTelemetry(cluster)...)
	allErrs = append(allErrs, validateTLS(cluster)...)
	networkErrs, networkWarnings := validateNetwork(cluster)
	allErrs = append(allErrs, networkErrs...)
	warnings = append(warnings, networkWarnings...)
	allErrs = append(allErrs, validateProfile(cluster)...)

	backupErrs, backupWarnings := validateBackup(cluster)
	allErrs = append(allErrs, backupErrs...)
	warnings = append(warnings, backupWarnings...)

	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(
			GroupVersion.WithKind("OpenBaoCluster").GroupKind(),
			cluster.Name,
			allErrs,
		)
	}

	return warnings, nil
}

// ValidateDelete validates OpenBaoCluster resources on delete. We currently do
// not enforce additional delete-time invariants beyond those handled by
// finalizers.
func (v *openBaoClusterValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cluster, ok := obj.(*OpenBaoCluster)
	if !ok {
		return nil, apierrors.NewBadRequest("expected OpenBaoCluster object for validation")
	}

	openBaoClusterWebhookLog.Info("validating delete", "name", cluster.Name, "namespace", cluster.Namespace)
	return nil, nil
}

// validateConfig performs allowlist-based validation of the user-supplied
// spec.config map. It enforces:
//   - Only allows configuration keys that are valid OpenBao parameters
//   - Excludes operator-managed configuration stanzas
//   - Basic sanity checks on value size and encoding.
//
// This uses an allowlist approach based on the official OpenBao configuration
// documentation (https://openbao.org/docs/configuration/), which is more secure
// than a blocklist as it prevents unknown or newly introduced configuration
// parameters from being accepted.
func validateConfig(cluster *OpenBaoCluster) field.ErrorList {
	config := cluster.Spec.Config
	if len(config) == 0 {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, configFieldPathConfig)
	var allErrs field.ErrorList

	if len(config) > maxConfigEntries {
		allErrs = append(allErrs, field.TooMany(path, len(config), maxConfigEntries))
	}

	// Allowlist of valid OpenBao configuration parameters that users can configure.
	// This list is based on the official OpenBao configuration documentation:
	// https://openbao.org/docs/configuration/
	//
	// Operator-managed parameters are excluded:
	// - listener (operator manages TLS listener configuration)
	// - storage (operator manages Raft storage)
	// - seal (operator manages static seal)
	// - api_addr (operator manages based on service configuration)
	// - cluster_addr (operator manages based on service configuration)
	// - node_id (operator manages via template)
	// - service_registration (operator manages if needed)
	// - initialize (operator manages via self-init config)
	// - ha_storage (not used, Raft has built-in HA)
	// - plugin (block type, not a simple string attribute)
	// - telemetry (block type, not a simple string attribute)
	// - audit (block type, not a simple string attribute)
	// - user_lockout (block type, not a simple string attribute)
	allowedKeys := map[string]struct{}{
		// Cluster configuration
		"cluster_name":       {},
		"disable_clustering": {},

		// Cache configuration
		"cache_size":    {},
		"disable_cache": {},

		// Plugin configuration
		"plugin_directory":         {},
		"plugin_file_uid":          {},
		"plugin_file_permissions":  {},
		"plugin_auto_download":     {},
		"plugin_auto_register":     {},
		"plugin_download_behavior": {},

		// Lease configuration
		"default_lease_ttl": {},
		"max_lease_ttl":     {},

		// Request configuration
		"default_max_request_duration": {},
		"detect_deadlocks":             {},

		// Feature flags
		"raw_storage_endpoint":            {},
		"introspection_endpoint":          {},
		"ui":                              {},
		"disable_standby_reads":           {},
		"imprecise_lease_role_tracking":   {},
		"unsafe_allow_api_audit_creation": {},
		"allow_audit_log_prefixing":       {},

		// Response headers
		"enable_response_header_hostname":     {},
		"enable_response_header_raft_node_id": {},

		// Logging configuration
		"log_level":            {},
		"log_format":           {},
		"log_file":             {},
		"log_rotate_duration":  {},
		"log_rotate_bytes":     {},
		"log_rotate_max_files": {},

		// Process configuration
		"pid_file": {},
	}

	for key, value := range config {
		keyPath := path.Key(key)

		if len(key) == 0 {
			allErrs = append(allErrs, field.Invalid(keyPath, key, "configuration key must not be empty"))
			continue
		}

		if len(key) > maxConfigKeyLength {
			allErrs = append(allErrs, field.TooLong(keyPath, key, maxConfigKeyLength))
		}

		if !configKeyPattern.MatchString(key) {
			allErrs = append(allErrs, field.Invalid(
				keyPath,
				key,
				"configuration keys must use snake_case with characters [a-z0-9_]",
			))
		}

		// Reject keys not in the allowlist
		if _, allowed := allowedKeys[key]; !allowed {
			allErrs = append(allErrs, field.Forbidden(
				keyPath,
				fmt.Sprintf("configuration key %q is not allowed. Only valid OpenBao configuration parameters may be specified. Operator-managed parameters (listener, storage, seal, api_addr, cluster_addr, node_id, service_registration, initialize) cannot be overridden.", key),
			))
		}

		valuePath := keyPath
		if len(value) == 0 {
			continue
		}

		if len(value) > maxConfigValueLength {
			allErrs = append(allErrs, field.TooLong(valuePath, "<redacted>", maxConfigValueLength))
		}

		if !utf8.ValidString(value) {
			allErrs = append(allErrs, field.Invalid(valuePath, "<non-utf8>", "configuration values must be valid UTF-8 text"))
		}
	}

	return allErrs
}

// validateInitContainer enforces the contract for the config init container.
// The operator requires an init container image to render config.hcl at runtime
// and does not support disabling the init container.
func validateInitContainer(cluster *OpenBaoCluster) field.ErrorList {
	path := field.NewPath(configFieldPathRoot, "initContainer")
	var allErrs field.ErrorList

	cfg := cluster.Spec.InitContainer
	if cfg == nil {
		allErrs = append(allErrs, field.Required(
			path,
			"initContainer configuration is required; the operator uses it to render config.hcl at runtime",
		))
		return allErrs
	}

	if !cfg.Enabled {
		allErrs = append(allErrs, field.Invalid(
			path.Child("enabled"),
			cfg.Enabled,
			"disabling the init container is not supported; the operator requires it to render config.hcl",
		))
	}

	if strings.TrimSpace(cfg.Image) == "" {
		allErrs = append(allErrs, field.Required(
			path.Child("image"),
			"initContainer.image is required and must be a non-empty image reference",
		))
	}

	return allErrs
}

// validateSelfInit performs validation of the self-initialization configuration.
// It enforces:
//   - Request name uniqueness.
//   - Request name syntax (must match ^[A-Za-z_][A-Za-z0-9_-]*$).
//   - Valid operation types.
//   - Non-empty path.
func validateSelfInit(cluster *OpenBaoCluster) field.ErrorList {
	selfInit := cluster.Spec.SelfInit
	if selfInit == nil || !selfInit.Enabled {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, configFieldPathSelfInit, "requests")
	var allErrs field.ErrorList

	if len(selfInit.Requests) > maxSelfInitRequests {
		allErrs = append(allErrs, field.TooMany(path, len(selfInit.Requests), maxSelfInitRequests))
	}

	// Track seen names to detect duplicates
	seenNames := make(map[string]int)

	validOperations := map[SelfInitOperation]struct{}{
		SelfInitOperationCreate: {},
		SelfInitOperationRead:   {},
		SelfInitOperationUpdate: {},
		SelfInitOperationDelete: {},
		SelfInitOperationList:   {},
	}

	for i, req := range selfInit.Requests {
		reqPath := path.Index(i)

		// Validate name
		namePath := reqPath.Child("name")
		if req.Name == "" {
			allErrs = append(allErrs, field.Required(namePath, "request name is required"))
		} else {
			if !selfInitRequestNameRegex.MatchString(req.Name) {
				allErrs = append(allErrs, field.Invalid(
					namePath,
					req.Name,
					"request name must match regex ^[A-Za-z_][A-Za-z0-9_-]*$",
				))
			}

			// Check for duplicate names
			if prevIdx, seen := seenNames[req.Name]; seen {
				allErrs = append(allErrs, field.Duplicate(namePath, fmt.Sprintf("request name %q is already used at index %d", req.Name, prevIdx)))
			} else {
				seenNames[req.Name] = i
			}
		}

		// Validate operation
		operationPath := reqPath.Child("operation")
		if req.Operation == "" {
			allErrs = append(allErrs, field.Required(operationPath, "operation is required"))
		} else if _, valid := validOperations[req.Operation]; !valid {
			allErrs = append(allErrs, field.NotSupported(
				operationPath,
				string(req.Operation),
				[]string{
					string(SelfInitOperationCreate),
					string(SelfInitOperationRead),
					string(SelfInitOperationUpdate),
					string(SelfInitOperationDelete),
					string(SelfInitOperationList),
				},
			))
		}

		// Validate path
		pathPath := reqPath.Child("path")
		if req.Path == "" {
			allErrs = append(allErrs, field.Required(pathPath, "API path is required"))
		} else if len(req.Path) > maxSelfInitRequestPath {
			allErrs = append(allErrs, field.TooLong(pathPath, req.Path, maxSelfInitRequestPath))
		}
	}

	return allErrs
}

// validateGateway performs validation of the Gateway API configuration.
// It enforces:
//   - Gateway reference name is provided when enabled.
//   - Hostname is provided when enabled.
func validateGateway(cluster *OpenBaoCluster) field.ErrorList {
	gateway := cluster.Spec.Gateway
	if gateway == nil || !gateway.Enabled {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, configFieldPathGateway)
	var allErrs field.ErrorList

	// Validate gateway reference
	gatewayRefPath := path.Child("gatewayRef")
	if gateway.GatewayRef.Name == "" {
		allErrs = append(allErrs, field.Required(gatewayRefPath.Child("name"), "Gateway reference name is required when Gateway is enabled"))
	}

	// Validate hostname
	hostnamePath := path.Child("hostname")
	if gateway.Hostname == "" {
		allErrs = append(allErrs, field.Required(hostnamePath, "hostname is required when Gateway is enabled"))
	}

	return allErrs
}

// validateBackup performs validation of the backup configuration.
// It enforces:
//   - Valid cron expression for schedule.
//   - Minimum schedule interval (15 minutes).
//   - Valid retention policy values.
//   - Required backup target fields.
//
// It returns warnings for:
//   - Schedules more frequent than 1 hour.
func validateBackup(cluster *OpenBaoCluster) (field.ErrorList, []string) {
	backup := cluster.Spec.Backup
	if backup == nil {
		return nil, nil
	}

	path := field.NewPath(configFieldPathRoot, configFieldPathBackup)
	var allErrs field.ErrorList
	var warnings []string

	// Validate schedule
	schedulePath := path.Child("schedule")
	if backup.Schedule == "" {
		allErrs = append(allErrs, field.Required(schedulePath, "backup schedule is required"))
	} else {
		// Parse the cron expression
		schedule, err := cronParser.Parse(backup.Schedule)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(schedulePath, backup.Schedule,
				fmt.Sprintf("invalid cron expression: %v", err)))
		} else {
			// Calculate schedule interval
			now := time.Now().UTC()
			next := schedule.Next(now)
			nextNext := schedule.Next(next)
			interval := nextNext.Sub(next)

			if interval < minBackupScheduleInterval {
				allErrs = append(allErrs, field.Invalid(schedulePath, backup.Schedule,
					fmt.Sprintf("backup schedule interval %v is less than minimum allowed %v", interval, minBackupScheduleInterval)))
			} else if interval < warnBackupScheduleInterval {
				warnings = append(warnings, fmt.Sprintf("backup schedule interval %v is less than recommended %v; frequent backups may impact cluster performance", interval, warnBackupScheduleInterval))
			}
		}
	}

	// Validate target
	targetPath := path.Child("target")
	if backup.Target.Endpoint == "" {
		allErrs = append(allErrs, field.Required(targetPath.Child("endpoint"), "backup target endpoint is required"))
	}
	if backup.Target.Bucket == "" {
		allErrs = append(allErrs, field.Required(targetPath.Child("bucket"), "backup target bucket is required"))
	}

	// Validate retention if configured
	if backup.Retention != nil {
		retentionPath := path.Child("retention")

		// Validate MaxCount is non-negative (enforced by CRD but double-check)
		if backup.Retention.MaxCount < 0 {
			allErrs = append(allErrs, field.Invalid(retentionPath.Child("maxCount"),
				backup.Retention.MaxCount, "maxCount must be non-negative"))
		}

		// Validate MaxAge is a valid duration
		if backup.Retention.MaxAge != "" {
			duration, err := time.ParseDuration(backup.Retention.MaxAge)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(retentionPath.Child("maxAge"),
					backup.Retention.MaxAge, fmt.Sprintf("invalid duration format: %v", err)))
			} else if duration <= 0 {
				allErrs = append(allErrs, field.Invalid(retentionPath.Child("maxAge"),
					backup.Retention.MaxAge, "maxAge must be a positive duration"))
			}
		}

		// Warn if no retention policy is effectively configured
		if backup.Retention.MaxCount == 0 && backup.Retention.MaxAge == "" {
			warnings = append(warnings, "retention policy is configured but both maxCount and maxAge are empty; no backups will be automatically deleted")
		}
	}

	return allErrs, warnings
}

// validateTLS performs validation of TLS configuration.
// It enforces:
//   - ACME configuration is required when Mode is ACME.
func validateTLS(cluster *OpenBaoCluster) field.ErrorList {
	if !cluster.Spec.TLS.Enabled {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, "tls")
	var allErrs field.ErrorList

	// If mode is ACME, ACME config must be provided
	if cluster.Spec.TLS.Mode == TLSModeACME {
		if cluster.Spec.TLS.ACME == nil {
			allErrs = append(allErrs, field.Required(
				path.Child("acme"),
				"ACME configuration is required when tls.mode is ACME",
			))
		} else {
			acmePath := path.Child("acme")
			if cluster.Spec.TLS.ACME.DirectoryURL == "" {
				allErrs = append(allErrs, field.Required(
					acmePath.Child("directoryURL"),
					"ACME directoryURL is required",
				))
			}
			if cluster.Spec.TLS.ACME.Domain == "" {
				allErrs = append(allErrs, field.Required(
					acmePath.Child("domain"),
					"ACME domain is required",
				))
			}
		}
	}

	return allErrs
}

// validateProfile enforces security profile requirements.
// TIGHTENED: No check for empty string. CRD default ensures this is either
// "Development" or "Hardened".
func validateProfile(cluster *OpenBaoCluster) field.ErrorList {
	// If Development, we return immediately (allow permissive mode).
	if cluster.Spec.Profile == ProfileDevelopment {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, "profile")
	var allErrs field.ErrorList

	// Logic for Hardened profile
	if cluster.Spec.Profile == ProfileHardened {
		// Hardened profile requirements

		// 1. TLS must be External OR ACME
		// Both ensure the Operator never possesses the private keys.
		if cluster.Spec.TLS.Mode != TLSModeExternal && cluster.Spec.TLS.Mode != TLSModeACME {
			allErrs = append(allErrs, field.Invalid(
				path,
				cluster.Spec.Profile,
				fmt.Sprintf("Hardened profile requires spec.tls.mode to be %q or %q, got %q", TLSModeExternal, TLSModeACME, cluster.Spec.TLS.Mode),
			))
		}

		// 2. Unseal must be external KMS (not static)
		unsealType := "static" // default
		if cluster.Spec.Unseal != nil {
			unsealType = cluster.Spec.Unseal.Type
		}
		if unsealType == "static" {
			allErrs = append(allErrs, field.Invalid(
				path,
				cluster.Spec.Profile,
				"Hardened profile requires external KMS unseal (awskms, gcpckms, azurekeyvault, or transit), static unseal is not allowed",
			))
		}

		// 3. SelfInit must be enabled
		if cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled {
			allErrs = append(allErrs, field.Invalid(
				path,
				cluster.Spec.Profile,
				"Hardened profile requires spec.selfInit.enabled to be true (root token must be auto-revoked)",
			))
		}

		// 4. Reject insecure unseal options
		if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Options != nil {
			if tlsSkipVerify, ok := cluster.Spec.Unseal.Options["tls_skip_verify"]; ok && tlsSkipVerify == "true" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath(configFieldPathRoot, "unseal", "options", "tls_skip_verify"),
					tlsSkipVerify,
					"Hardened profile does not allow tls_skip_verify=true in unseal options",
				))
			}
		}
	}

	return allErrs
}

// validateAudit performs validation of audit device configurations.
func validateAudit(cluster *OpenBaoCluster) field.ErrorList {
	auditDevices := cluster.Spec.Audit
	if len(auditDevices) == 0 {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, "audit")
	var allErrs field.ErrorList

	// Track paths to ensure uniqueness
	seenPaths := make(map[string]int)

	for i, device := range auditDevices {
		devicePath := path.Index(i)

		// Validate type
		if device.Type == "" {
			allErrs = append(allErrs, field.Required(devicePath.Child("type"), "audit device type is required"))
		}

		// Validate path
		if device.Path == "" {
			allErrs = append(allErrs, field.Required(devicePath.Child("path"), "audit device path is required"))
		} else {
			// Check for duplicate paths
			if prevIdx, seen := seenPaths[device.Path]; seen {
				allErrs = append(allErrs, field.Duplicate(devicePath.Child("path"),
					fmt.Sprintf("audit device path %q is already used at index %d", device.Path, prevIdx)))
			} else {
				seenPaths[device.Path] = i
			}
		}

		// Validate options if provided
		if device.Options != nil && len(device.Options.Raw) > 0 {
			var options map[string]interface{}
			if err := json.Unmarshal(device.Options.Raw, &options); err != nil {
				allErrs = append(allErrs, field.Invalid(devicePath.Child("options"), "<invalid-json>",
					fmt.Sprintf("audit device options must be valid JSON: %v", err)))
			}
		}
	}

	return allErrs
}

// validatePlugins performs validation of plugin configurations.
func validatePlugins(cluster *OpenBaoCluster) field.ErrorList {
	plugins := cluster.Spec.Plugins
	if len(plugins) == 0 {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, "plugins")
	var allErrs field.ErrorList

	// Track (type, name, version) tuples to ensure uniqueness
	seenPlugins := make(map[string]int)

	for i, plugin := range plugins {
		pluginPath := path.Index(i)

		// Validate type
		if plugin.Type == "" {
			allErrs = append(allErrs, field.Required(pluginPath.Child("type"), "plugin type is required"))
		}

		// Validate name
		if plugin.Name == "" {
			allErrs = append(allErrs, field.Required(pluginPath.Child("name"), "plugin name is required"))
		}

		// Validate that either image or command is set (but not both)
		if plugin.Image == "" && plugin.Command == "" {
			allErrs = append(allErrs, field.Required(pluginPath.Child("image"),
				"either image or command must be specified for plugin"))
		}
		if plugin.Image != "" && plugin.Command != "" {
			allErrs = append(allErrs, field.Invalid(pluginPath.Child("image"), plugin.Image,
				"image and command cannot both be specified for plugin"))
		}

		// Validate version
		if plugin.Version == "" {
			allErrs = append(allErrs, field.Required(pluginPath.Child("version"), "plugin version is required"))
		}

		// Validate binaryName
		if plugin.BinaryName == "" {
			allErrs = append(allErrs, field.Required(pluginPath.Child("binaryName"), "plugin binaryName is required"))
		}

		// Validate sha256sum
		if plugin.SHA256Sum == "" {
			allErrs = append(allErrs, field.Required(pluginPath.Child("sha256sum"), "plugin sha256sum is required"))
		} else if len(plugin.SHA256Sum) != 64 {
			allErrs = append(allErrs, field.Invalid(pluginPath.Child("sha256sum"), plugin.SHA256Sum,
				"sha256sum must be exactly 64 hexadecimal characters"))
		}

		// Check for duplicate (type, name, version) combinations
		if plugin.Type != "" && plugin.Name != "" && plugin.Version != "" {
			key := fmt.Sprintf("%s:%s:%s", plugin.Type, plugin.Name, plugin.Version)
			if prevIdx, seen := seenPlugins[key]; seen {
				allErrs = append(allErrs, field.Duplicate(pluginPath,
					fmt.Sprintf("plugin with type %q, name %q, and version %q is already defined at index %d",
						plugin.Type, plugin.Name, plugin.Version, prevIdx)))
			} else {
				seenPlugins[key] = i
			}
		}
	}

	return allErrs
}

// validateTelemetry performs validation of telemetry configuration.
func validateTelemetry(cluster *OpenBaoCluster) field.ErrorList {
	telemetry := cluster.Spec.Telemetry
	if telemetry == nil {
		return nil
	}

	path := field.NewPath(configFieldPathRoot, "telemetry")
	var allErrs field.ErrorList

	// Validate usage gauge period if provided
	if telemetry.UsageGaugePeriod != "" && telemetry.UsageGaugePeriod != "none" {
		if _, err := time.ParseDuration(telemetry.UsageGaugePeriod); err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("usageGaugePeriod"), telemetry.UsageGaugePeriod,
				fmt.Sprintf("invalid duration format: %v", err)))
		}
	}

	// Validate maximum gauge cardinality if provided
	if telemetry.MaximumGaugeCardinality != nil && *telemetry.MaximumGaugeCardinality < 0 {
		allErrs = append(allErrs, field.Invalid(path.Child("maximumGaugeCardinality"),
			*telemetry.MaximumGaugeCardinality, "maximum gauge cardinality must be non-negative"))
	}

	// Validate lease metrics epsilon if provided
	if telemetry.LeaseMetricsEpsilon != "" {
		if _, err := time.ParseDuration(telemetry.LeaseMetricsEpsilon); err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("leaseMetricsEpsilon"), telemetry.LeaseMetricsEpsilon,
				fmt.Sprintf("invalid duration format: %v", err)))
		}
	}

	// Validate prometheus retention time if provided
	if telemetry.PrometheusRetentionTime != "" {
		if _, err := time.ParseDuration(telemetry.PrometheusRetentionTime); err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("prometheusRetentionTime"), telemetry.PrometheusRetentionTime,
				fmt.Sprintf("invalid duration format: %v", err)))
		}
	}

	return allErrs
}

func validateNetwork(cluster *OpenBaoCluster) (field.ErrorList, admission.Warnings) {
	var allErrs field.ErrorList
	var warnings admission.Warnings

	if cluster.Spec.Network == nil {
		return allErrs, warnings
	}

	if strings.TrimSpace(cluster.Spec.Network.APIServerCIDR) != "" {
		cidrPath := field.NewPath(configFieldPathRoot, "network", "apiServerCIDR")
		rawCIDR := strings.TrimSpace(cluster.Spec.Network.APIServerCIDR)

		_, ipNet, err := net.ParseCIDR(rawCIDR)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(cidrPath, rawCIDR, fmt.Sprintf("must be a valid CIDR: %v", err)))
		} else {
			canonical := ipNet.String()
			if canonical != rawCIDR {
				warnings = append(warnings, fmt.Sprintf("spec.network.apiServerCIDR normalized from %q to %q", rawCIDR, canonical))
			}
		}
	}

	if len(cluster.Spec.Network.APIServerEndpointIPs) > 0 {
		path := field.NewPath(configFieldPathRoot, "network", "apiServerEndpointIPs")
		seen := make(map[string]struct{}, len(cluster.Spec.Network.APIServerEndpointIPs))
		for i, rawIP := range cluster.Spec.Network.APIServerEndpointIPs {
			ip := strings.TrimSpace(rawIP)
			if ip == "" {
				allErrs = append(allErrs, field.Invalid(path.Index(i), rawIP, "must not be empty"))
				continue
			}

			parsed := net.ParseIP(ip)
			if parsed == nil {
				allErrs = append(allErrs, field.Invalid(path.Index(i), rawIP, "must be a valid IP address"))
				continue
			}

			canonical := parsed.String()
			if canonical != ip {
				warnings = append(warnings, fmt.Sprintf("spec.network.apiServerEndpointIPs[%d] normalized from %q to %q", i, ip, canonical))
			}

			if _, ok := seen[canonical]; ok {
				allErrs = append(allErrs, field.Duplicate(path.Index(i), rawIP))
				continue
			}
			seen[canonical] = struct{}{}
		}
	}

	return allErrs, warnings
}
