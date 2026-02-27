package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func (r *OpenBaoClusterReconciler) updateStatusForPaused(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	now := metav1.Now()

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "Reconciliation is paused; availability is not being evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "Cluster is paused; no new degradation has been evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "TLS readiness is not being evaluated while reconciliation is paused",
	})

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")
	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForProfileNotSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	now := metav1.Now()
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "TLS readiness is not being evaluated until spec.profile is set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "Cluster cannot be considered production-ready until spec.profile is explicitly set",
	})

	if err := r.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}
