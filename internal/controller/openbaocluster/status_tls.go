package openbaocluster

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
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
			Reason:             reasonUnknown,
			Message:            "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness",
		})
		return
	}

	// Check for CA TLS secret first. Day-2 workflows depend on the cluster trust
	// bundle, not only the leaf certificate.
	caSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixTLSCA,
	}, caSecret); err != nil {
		if apierrors.IsNotFound(err) {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionTLSReady),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonTLSSecretMissing,
				Message:            "CA TLS Secret is not present yet",
			})
			return
		}
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonUnknown,
			Message:            "Failed to get CA TLS secret",
		})
		return
	}
	if err := openbaotls.ValidateCABundle(caSecret.Data["ca.crt"]); err != nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonTLSSecretInvalid,
			Message:            "CA TLS Secret is invalid: " + err.Error(),
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
			Reason:             reasonUnknown,
			Message:            "Failed to get TLS secret",
		})
		return
	}

	if tlsMode == openbaov1alpha1.TLSModeExternal {
		if err := openbaotls.ValidateExternalServerSecret(cluster, caSecret, serverSecret); err != nil {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionTLSReady),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonTLSSecretInvalid,
				Message:            "External TLS assets are invalid: " + err.Error(),
			})
			return
		}
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonReady,
			Message:            "TLS assets are provisioned and valid",
		})
		return
	}

	if _, err := openbaotls.ValidateServerSecret(serverSecret); err == nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             reasonReady,
			Message:            "TLS assets are provisioned",
		})
	} else {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonTLSSecretInvalid,
			Message:            "Server TLS Secret is invalid: " + err.Error(),
		})
	}
}
