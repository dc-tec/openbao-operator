package resourcelock

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	managedByLabelKey      = "app.kubernetes.io/managed-by"
	managedByLabelValue    = "openbao-operator"
	maintenanceAnnotation  = "openbao.org/maintenance"
	serviceAccountPrefix   = "system:serviceaccount:"
	labelAppInstanceKey    = "app.kubernetes.io/instance"
	kubeSystemGroup        = "system:serviceaccounts:kube-system"
	adminGroupsEnv         = "OPENBAO_BREAKGLASS_ADMIN_GROUPS"
	systemSAAllowlistEnv   = "OPENBAO_SYSTEM_SA_ALLOWLIST"
	certManagerSAEnv       = "OPENBAO_CERTMANAGER_SA_ALLOWLIST"
	ingressControllerSAEnv = "OPENBAO_INGRESS_SA_ALLOWLIST"
)

var openBaoServiceRegistrationLabels = map[string]struct{}{
	"openbao-active":       {},
	"openbao-initialized":  {},
	"openbao-sealed":       {},
	"openbao-perf-standby": {},
	"openbao-version":      {},
}

const labelOpenBaoClusterKey = "openbao.org/cluster"

// ResourceLockValidator enforces the resource locking decision matrix for
// OpenBao-managed child resources. It is intentionally strict: only the
// operator, a small set of system controllers, approved external controllers,
// and break-glass administrators may mutate managed resources.
type ResourceLockValidator struct {
	logger                logr.Logger
	decoder               admission.Decoder
	operatorIdentity      string
	systemSAUsernames     map[string]struct{}
	certManagerSAs        map[string]struct{}
	ingressControllerSAs  map[string]struct{}
	breakGlassAdminGroups map[string]struct{}
}

// NewValidator constructs a ResourceLockValidator. It auto-detects the
// operator ServiceAccount from POD_NAMESPACE and OPERATOR_SERVICE_ACCOUNT_NAME
// and parses optional allowlists from environment variables.
func NewValidator(logger logr.Logger) *ResourceLockValidator {
	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		ns = "openbao-operator-system"
	}

	sa := os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME")
	if sa == "" {
		sa = "openbao-operator-controller"
	}

	operatorIdentity := serviceAccountPrefix + ns + ":" + sa

	v := &ResourceLockValidator{
		logger:                logger.WithName("resource-lock"),
		operatorIdentity:      operatorIdentity,
		systemSAUsernames:     parseServiceAccountAllowlist(systemSAAllowlistEnv),
		certManagerSAs:        parseServiceAccountAllowlist(certManagerSAEnv),
		ingressControllerSAs:  parseServiceAccountAllowlist(ingressControllerSAEnv),
		breakGlassAdminGroups: parseGroupAllowlist(adminGroupsEnv),
	}

	v.logger.Info("Resource lock validator configured",
		"operator_identity", operatorIdentity,
		"system_sa_allowlist_count", len(v.systemSAUsernames),
		"certmanager_sa_allowlist_count", len(v.certManagerSAs),
		"ingress_sa_allowlist_count", len(v.ingressControllerSAs),
		"breakglass_admin_groups_count", len(v.breakGlassAdminGroups))

	return v
}

// Handle evaluates admission requests for UPDATE/DELETE of managed resources.
func (v *ResourceLockValidator) Handle(_ context.Context, req admission.Request) admission.Response {
	// Only enforce on UPDATE/DELETE; CREATE is allowed to avoid bootstrap issues.
	if req.Operation != admissionv1.Update && req.Operation != admissionv1.Delete {
		return admission.Allowed("operation not subject to resource lock")
	}

	// Allow operator controller unconditionally.
	if req.UserInfo.Username == v.operatorIdentity {
		return admission.Allowed("operator controller authorized")
	}

	// Determine if this resource is managed by the OpenBao operator.
	isManaged, metaObj, err := v.isManagedResource(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if !isManaged {
		return admission.Allowed("resource not managed by OpenBao operator")
	}

	// At this point, the resource is managed/locked.

	// Allow kubelet nodes to delete managed Pods. The kubelet is responsible
	// for pod lifecycle and already has effective control over the processes
	// running on the node. This exception is scoped narrowly to DELETE on
	// Pod resources and does not grant broader mutation capabilities.
	if req.Operation == admissionv1.Delete &&
		req.Kind.Group == "" &&
		req.Kind.Kind == "Pod" &&
		strings.HasPrefix(req.UserInfo.Username, "system:node:") {
		return admission.Allowed("kubelet node authorized to delete managed pod")
	}

	// Allow kube-system controllers via group or explicit SA allowlist.
	if v.isKubeSystemController(req.UserInfo) {
		return admission.Allowed("system controller authorized")
	}

	// Allow cert-manager when rotating TLS Secrets or ConfigMaps.
	if v.isCertManager(req.UserInfo) {
		return admission.Allowed("cert-manager authorized")
	}

	// Allow ingress/gateway controllers to update status subresource.
	if req.SubResource == "status" && v.isIngressController(req.UserInfo) {
		return admission.Allowed("ingress or gateway controller status update authorized")
	}

	// For UPDATE operations, allow adding/modifying the maintenance annotation itself.
	// This enables users to activate maintenance mode without being blocked.
	if req.Operation == admissionv1.Update {
		if v.isOpenBaoServiceRegistrationPodUpdate(req) {
			return admission.Allowed("openbao service registration label update allowed")
		}

		if v.isOnlyMaintenanceAnnotationChange(req) && v.isBreakGlassAdmin(req.UserInfo) {
			v.logger.Info("allowing maintenance annotation update",
				"user", req.UserInfo.Username,
				"resource", fmt.Sprintf("%s/%s", req.Namespace, req.Name))
			return admission.Allowed("maintenance annotation update allowed for administrator")
		}
	}

	// Break-glass: allow cluster administrators when maintenance annotation is set.
	if v.isMaintenanceMode(metaObj) && v.isBreakGlassAdmin(req.UserInfo) {
		v.logger.Info("break-glass access granted",
			"user", req.UserInfo.Username,
			"resource", fmt.Sprintf("%s/%s", req.Namespace, req.Name),
			"operation", string(req.Operation))
		return admission.Allowed("maintenance mode active for administrator")
	}

	// Deny all other direct mutations.
	message := fmt.Sprintf("direct modification of OpenBao-managed resource %s/%s by %s is prohibited; "+
		"modify the parent OpenBaoCluster or OpenBaoTenant instead",
		req.Namespace, req.Name, req.UserInfo.Username)

	v.logger.Info("blocked unauthorized mutation",
		"user", req.UserInfo.Username,
		"resource", fmt.Sprintf("%s/%s", req.Namespace, req.Name),
		"operation", string(req.Operation))

	return admission.Denied(message)
}

func (v *ResourceLockValidator) isOpenBaoServiceRegistrationPodUpdate(req admission.Request) bool {
	if req.Operation != admissionv1.Update || req.SubResource != "" {
		return false
	}

	clusterName, ok := v.clusterNameFromServiceAccount(req)
	if !ok {
		return false
	}

	oldPod := &corev1.Pod{}
	newPod := &corev1.Pod{}

	if err := v.decoder.DecodeRaw(req.OldObject, oldPod); err != nil {
		return false
	}
	if err := v.decoder.DecodeRaw(req.Object, newPod); err != nil {
		return false
	}

	if newPod.Namespace != req.Namespace || newPod.Name != req.Name {
		return false
	}

	if !isPodForCluster(newPod, clusterName) {
		return false
	}

	// Enforce that only service registration labels change. Any other change is denied.
	if !apiequality.Semantic.DeepEqual(oldPod.Spec, newPod.Spec) {
		return false
	}
	if !mapsEqual(oldPod.Annotations, newPod.Annotations) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(oldPod.OwnerReferences, newPod.OwnerReferences) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(oldPod.Finalizers, newPod.Finalizers) {
		return false
	}

	oldStaticLabels := stripServiceRegistrationLabels(oldPod.Labels)
	newStaticLabels := stripServiceRegistrationLabels(newPod.Labels)
	if !mapsEqual(oldStaticLabels, newStaticLabels) {
		return false
	}

	return true
}

func (v *ResourceLockValidator) clusterNameFromServiceAccount(req admission.Request) (string, bool) {
	expectedPrefix := serviceAccountPrefix + req.Namespace + ":"
	if !strings.HasPrefix(req.UserInfo.Username, expectedPrefix) {
		return "", false
	}

	saName := strings.TrimPrefix(req.UserInfo.Username, expectedPrefix)
	if !strings.HasSuffix(saName, "-serviceaccount") {
		return "", false
	}

	clusterName := strings.TrimSuffix(saName, "-serviceaccount")
	if strings.TrimSpace(clusterName) == "" {
		return "", false
	}

	return clusterName, true
}

func isPodForCluster(pod *corev1.Pod, clusterName string) bool {
	if pod == nil {
		return false
	}

	if pod.Labels != nil {
		if strings.TrimSpace(pod.Labels[labelAppInstanceKey]) == clusterName {
			return true
		}
		if strings.TrimSpace(pod.Labels[labelOpenBaoClusterKey]) == clusterName {
			return true
		}
	}

	return strings.HasPrefix(pod.Name, clusterName+"-")
}

func stripServiceRegistrationLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range labels {
		if _, allowed := openBaoServiceRegistrationLabels[k]; allowed {
			continue
		}
		result[k] = v
	}
	return result
}

// InjectDecoder implements admission.DecoderInjector.
func (v *ResourceLockValidator) InjectDecoder(d *admission.Decoder) error {
	if d == nil {
		return fmt.Errorf("decoder must not be nil")
	}
	v.decoder = *d
	return nil
}

// isManagedResource determines whether the admission request targets a
// resource managed by the OpenBao operator, based on labels and owner
// references.
func (v *ResourceLockValidator) isManagedResource(req admission.Request) (bool, metav1.Object, error) {
	obj := &metav1.PartialObjectMetadata{}
	if req.Operation == admissionv1.Delete {
		if err := v.decoder.DecodeRaw(req.OldObject, obj); err != nil {
			return false, nil, fmt.Errorf("failed to decode old object metadata: %w", err)
		}
	} else {
		if err := v.decoder.DecodeRaw(req.Object, obj); err != nil {
			return false, nil, fmt.Errorf("failed to decode object metadata: %w", err)
		}
	}

	labels := obj.GetLabels()
	if val, ok := labels[managedByLabelKey]; ok && val == managedByLabelValue {
		return true, obj, nil
	}

	for _, owner := range obj.OwnerReferences {
		if owner.Kind == "OpenBaoCluster" && strings.HasPrefix(owner.APIVersion, "openbao.org/") {
			return true, obj, nil
		}
	}

	return false, obj, nil
}

func (v *ResourceLockValidator) isMaintenanceMode(obj metav1.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	val, ok := annotations[maintenanceAnnotation]
	return ok && val == "true"
}

// isOnlyMaintenanceAnnotationChange checks if an UPDATE operation only modifies
// the maintenance annotation. This allows users to enable maintenance mode
// without being blocked by the webhook.
func (v *ResourceLockValidator) isOnlyMaintenanceAnnotationChange(req admission.Request) bool {
	if req.Operation != admissionv1.Update {
		return false
	}

	oldObj := &metav1.PartialObjectMetadata{}
	newObj := &metav1.PartialObjectMetadata{}

	if err := v.decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
		return false
	}
	if err := v.decoder.DecodeRaw(req.Object, newObj); err != nil {
		return false
	}

	oldAnnotations := oldObj.GetAnnotations()
	newAnnotations := newObj.GetAnnotations()

	// If old annotations is nil, treat as empty map
	if oldAnnotations == nil {
		oldAnnotations = make(map[string]string)
	}
	if newAnnotations == nil {
		newAnnotations = make(map[string]string)
	}

	// Check if the only difference is the maintenance annotation
	allKeys := make(map[string]struct{})
	for k := range oldAnnotations {
		allKeys[k] = struct{}{}
	}
	for k := range newAnnotations {
		allKeys[k] = struct{}{}
	}

	for key := range allKeys {
		if key == maintenanceAnnotation {
			// Maintenance annotation change is allowed
			continue
		}
		// Any other annotation change is not allowed
		if oldAnnotations[key] != newAnnotations[key] {
			return false
		}
	}

	// Check if labels changed
	if !mapsEqual(oldObj.GetLabels(), newObj.GetLabels()) {
		return false
	}

	// Check if ownerReferences changed
	oldOwners := oldObj.GetOwnerReferences()
	newOwners := newObj.GetOwnerReferences()
	if len(oldOwners) != len(newOwners) {
		return false
	}
	// Create maps for easier comparison
	oldOwnerMap := make(map[string]metav1.OwnerReference)
	for _, owner := range oldOwners {
		key := fmt.Sprintf("%s/%s/%s", owner.APIVersion, owner.Kind, owner.Name)
		oldOwnerMap[key] = owner
	}
	for _, owner := range newOwners {
		key := fmt.Sprintf("%s/%s/%s", owner.APIVersion, owner.Kind, owner.Name)
		oldOwner, exists := oldOwnerMap[key]
		if !exists {
			return false
		}
		// Compare UID to ensure it's the same owner
		if oldOwner.UID != owner.UID {
			return false
		}
	}

	// If we get here, only the maintenance annotation changed
	return true
}

// mapsEqual compares two maps for equality
func mapsEqual(a, b map[string]string) bool {
	if a == nil {
		a = map[string]string{}
	}
	if b == nil {
		b = map[string]string{}
	}
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (v *ResourceLockValidator) isKubeSystemController(user authenticationv1.UserInfo) bool {
	for _, group := range user.Groups {
		if group == kubeSystemGroup {
			return true
		}
	}

	if _, ok := v.systemSAUsernames[user.Username]; ok {
		return true
	}

	return false
}

func (v *ResourceLockValidator) isCertManager(user authenticationv1.UserInfo) bool {
	if _, ok := v.certManagerSAs[user.Username]; ok {
		return true
	}
	return false
}

func (v *ResourceLockValidator) isIngressController(user authenticationv1.UserInfo) bool {
	if _, ok := v.ingressControllerSAs[user.Username]; ok {
		return true
	}
	return false
}

func (v *ResourceLockValidator) isBreakGlassAdmin(user authenticationv1.UserInfo) bool {
	if len(v.breakGlassAdminGroups) == 0 {
		return false
	}

	for _, group := range user.Groups {
		if _, ok := v.breakGlassAdminGroups[group]; ok {
			return true
		}
	}

	return false
}

func parseServiceAccountAllowlist(envKey string) map[string]struct{} {
	value := os.Getenv(envKey)
	result := make(map[string]struct{})
	if strings.TrimSpace(value) == "" {
		return result
	}

	entries := strings.Split(value, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) != 2 {
			continue
		}
		ns := strings.TrimSpace(parts[0])
		sa := strings.TrimSpace(parts[1])
		if ns == "" || sa == "" {
			continue
		}
		username := serviceAccountPrefix + ns + ":" + sa
		result[username] = struct{}{}
	}

	return result
}

func parseGroupAllowlist(envKey string) map[string]struct{} {
	value := os.Getenv(envKey)
	result := make(map[string]struct{})
	if strings.TrimSpace(value) == "" {
		return result
	}

	entries := strings.Split(value, ",")
	for _, entry := range entries {
		group := strings.TrimSpace(entry)
		if group == "" {
			continue
		}
		result[group] = struct{}{}
	}

	return result
}
