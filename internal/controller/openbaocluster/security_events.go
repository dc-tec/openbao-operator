package openbaocluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func (r *OpenBaoClusterReconciler) emitSecurityWarningEvents(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if r.Recorder == nil {
		return nil
	}

	now := time.Now().UTC()
	annotationUpdates := make(map[string]string)

	// Helper to check and potentially add warning
	maybeWarn := func(annotationKey, reason, message string) {
		if !shouldEmitSecurityWarning(cluster.Annotations, annotationKey, now) {
			return
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, reason, "%s", message)
		annotationUpdates[annotationKey] = now.Format(time.RFC3339Nano)
	}

	if cluster.Spec.Profile == "" {
		maybeWarn(constants.AnnotationLastProfileNotSetWarning, ReasonProfileNotSet, "spec.profile is not set; reconciliation is blocked until spec.profile is explicitly set to Hardened or Development")
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		maybeWarn(constants.AnnotationLastDevelopmentWarning, ReasonDevelopmentProfile, "Cluster is using Development profile; this is not suitable for production")
	}

	if isStaticUnseal(cluster) {
		maybeWarn(constants.AnnotationLastStaticUnsealWarning, ReasonStaticUnsealInUse, "Cluster is using static auto-unseal; the unseal key is stored in a Kubernetes Secret and is not suitable for production without etcd encryption at rest and strict RBAC")
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		maybeWarn(constants.AnnotationLastRootTokenWarning, ReasonRootTokenStored, "SelfInit is disabled; the operator will store a root token in a Kubernetes Secret during bootstrap, which is not suitable for production")
	}

	if len(annotationUpdates) == 0 {
		return nil
	}

	// Prepare SSA patch for annotations
	// We only include the annotations we want to update. SSA will merge these.
	// We must ensure the object has proper TypeMeta for SSA.
	patch := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        cluster.Name,
			Namespace:   cluster.Namespace,
			Annotations: annotationUpdates,
		},
	}

	// Apply partial patch with specific field owner for security events
	patchOpts := []client.PatchOption{
		client.FieldOwner("openbao-security-events"),
		client.ForceOwnership,
	}

	if err := r.Patch(ctx, patch, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to persist security warning timestamps on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	// Update the in-memory object to reflect the changes so subsequent logic sees them
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	for k, v := range annotationUpdates {
		cluster.Annotations[k] = v
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

	return now.Sub(last) >= constants.SecurityWarningInterval
}
