package openbaocluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

func (r *OpenBaoClusterReconciler) emitSecurityWarningEvents(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if r.Recorder == nil {
		return nil
	}

	now := time.Now().UTC()
	annotationUpdates := make(map[string]string)

	// Helper to check and potentially add warning
	maybeEvent := func(annotationKey, eventType, reason, message string) {
		if !shouldEmitSecurityWarning(cluster.Annotations, annotationKey, now) {
			return
		}
		r.Recorder.Eventf(cluster, nil, eventType, reason, reason, "%s", message)
		annotationUpdates[annotationKey] = now.Format(time.RFC3339Nano)
	}
	maybeWarn := func(annotationKey, reason, message string) {
		maybeEvent(annotationKey, corev1.EventTypeWarning, reason, message)
	}

	if cluster.Spec.Profile == "" {
		maybeWarn(annotationLastProfileNotSetWarning, ReasonProfileNotSet, "spec.profile is not set; reconciliation is blocked until spec.profile is explicitly set to Hardened or Development")
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		maybeWarn(annotationLastDevelopmentWarning, ReasonDevelopmentProfile, "Cluster is using Development profile; this is not suitable for production")
	}

	if admission.UnsafeAdmissionDisabled() {
		maybeWarn(annotationLastUnsafeAdmissionWarning, ReasonUnsafeAdmissionDisabled, "Unsafe admission mode is active; required admission guardrails are not enforced and Hardened clusters are not production-ready")
	}

	if appopenbaocluster.IsStaticUnseal(cluster) {
		maybeWarn(annotationLastStaticUnsealWarning, ReasonStaticUnsealInUse, "Cluster is using static auto-unseal; the unseal key is stored in a Kubernetes Secret and is not suitable for production without etcd encryption at rest and strict RBAC")
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		maybeWarn(annotationLastRootTokenWarning, ReasonRootTokenStored, "SelfInit is disabled; the operator will store a root token in a Kubernetes Secret during bootstrap, which is not suitable for production")
	}

	if message, ok := appopenbaocluster.AmbientCloudUnsealIdentityMessage(cluster); ok {
		maybeEvent(
			annotationLastAmbientUnsealNote,
			corev1.EventTypeNormal,
			ReasonAmbientUnsealIdentity,
			message,
		)
	}

	if len(annotationUpdates) == 0 {
		return nil
	}

	// Use JSON MergePatch for annotations.
	// SSA on the main resource (not status subresource) triggers full CRD validation
	// which fails on required spec fields. MergePatch only touches the specified fields.
	patchData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": annotationUpdates,
		},
	}

	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("failed to marshal annotation patch: %w", err)
	}

	if err := r.Patch(ctx, cluster, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to persist security warning timestamps on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.V(1).Info("Emitted security warning events", "count", len(annotationUpdates))
	return nil
}

func shouldEmitSecurityWarning(annotations map[string]string, annotationKey string, now time.Time) bool {
	if len(annotations) == 0 {
		return true
	}

	raw, ok := annotations[annotationKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return true
	}

	last, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return true
	}

	return now.Sub(last) >= securityWarningInterval
}
