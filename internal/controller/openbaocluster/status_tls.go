package openbaocluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// setTLSReadyCondition evaluates and sets the TLSReady condition.
func (r *OpenBaoClusterReconciler) setTLSReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	now := metav1.Now()

	if !cluster.Spec.TLS.Enabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonDisabled,
			Message:            "TLS is disabled",
		})
		return
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             constants.ReasonUnknown,
			Message:            "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness",
		})
		return
	}

	// Check for server TLS secret.
	serverSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixTLSServer,
	}, serverSecret); err != nil {
		if apierrors.IsNotFound(err) {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionTLSReady),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonTLSSecretMissing,
				Message:            "Server TLS Secret is not present yet",
			})
			return
		}
		// For other errors, mark as unknown.
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             constants.ReasonUnknown,
			Message:            "Failed to get TLS secret",
		})
		return
	}

	hasCert := len(serverSecret.Data["tls.crt"]) > 0
	hasKey := len(serverSecret.Data["tls.key"]) > 0
	if hasCert && hasKey {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             constants.ReasonReady,
			Message:            "TLS assets are provisioned",
		})
	} else {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonTLSSecretInvalid,
			Message:            "Server TLS Secret is missing required keys (tls.crt/tls.key)",
		})
	}
}
