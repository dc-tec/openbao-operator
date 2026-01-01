package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
	openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/revision"
)

func (r *OpenBaoClusterReconciler) updateStatusForPaused(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

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

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForProfileNotSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	original := cluster.DeepCopy()

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

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}

//nolint:gocyclo // Status computation is intentionally explicit to keep reconciliation readable and auditable.
func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

	findActiveBackupJob := func() (string, bool, error) {
		jobList := &batchv1.JobList{}
		labelSelector := labels.SelectorFromSet(map[string]string{
			constants.LabelAppInstance:      cluster.Name,
			constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
			constants.LabelOpenBaoCluster:   cluster.Name,
			constants.LabelOpenBaoComponent: constants.ComponentBackup,
		})
		if err := r.List(ctx, jobList,
			client.InNamespace(cluster.Namespace),
			client.MatchingLabelsSelector{Selector: labelSelector},
		); err != nil {
			return "", false, fmt.Errorf("failed to list backup Jobs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}

		for i := range jobList.Items {
			job := &jobList.Items[i]
			if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
				return job.Name, true, nil
			}
		}
		return "", false, nil
	}

	upgradeFailed := false
	rollingUpgradeInProgress := cluster.Status.Upgrade != nil
	if rollingUpgradeInProgress && cluster.Status.Upgrade.LastErrorReason != "" {
		upgradeFailed = true
	}

	blueGreenInProgress := false
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase != "" && cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		blueGreenInProgress = true
	}

	upgradeInProgress := (rollingUpgradeInProgress && !upgradeFailed) || blueGreenInProgress

	backupJobName, backupInProgress, err := findActiveBackupJob()
	if err != nil {
		return ctrl.Result{}, err
	}

	// Set TLSReady early so it is present even when we return early to requeue
	// due to StatefulSet status propagation delays.
	tlsNow := metav1.Now()
	if !cluster.Spec.TLS.Enabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: tlsNow,
			Reason:             "Disabled",
			Message:            "TLS is disabled",
		})
	} else {
		tlsMode := cluster.Spec.TLS.Mode
		if tlsMode == "" {
			tlsMode = openbaov1alpha1.TLSModeOperatorManaged
		}

		switch tlsMode {
		case openbaov1alpha1.TLSModeACME:
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionTLSReady),
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: tlsNow,
				Reason:             constants.ReasonUnknown,
				Message:            "TLS is managed by OpenBao via ACME; the operator does not evaluate certificate readiness",
			})
		default:
			serverSecret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      cluster.Name + constants.SuffixTLSServer,
			}, serverSecret); err != nil {
				if !apierrors.IsNotFound(err) {
					return ctrl.Result{}, fmt.Errorf("failed to get server TLS Secret for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
				}

				meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
					Type:               string(openbaov1alpha1.ConditionTLSReady),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: cluster.Generation,
					LastTransitionTime: tlsNow,
					Reason:             ReasonTLSSecretMissing,
					Message:            "Server TLS Secret is not present yet",
				})
			} else {
				hasCert := len(serverSecret.Data["tls.crt"]) > 0
				hasKey := len(serverSecret.Data["tls.key"]) > 0
				if hasCert && hasKey {
					meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
						Type:               string(openbaov1alpha1.ConditionTLSReady),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: cluster.Generation,
						LastTransitionTime: tlsNow,
						Reason:             constants.ReasonReady,
						Message:            "TLS assets are provisioned",
					})
				} else {
					meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
						Type:               string(openbaov1alpha1.ConditionTLSReady),
						Status:             metav1.ConditionFalse,
						ObservedGeneration: cluster.Generation,
						LastTransitionTime: tlsNow,
						Reason:             ReasonTLSSecretInvalid,
						Message:            "Server TLS Secret is missing required keys (tls.crt/tls.key)",
					})
				}
			}
		}
	}

	// Fetch the StatefulSet to get ready replicas count
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	activeRevision := ""
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		activeRevision = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			activeRevision = cluster.Status.BlueGreen.BlueRevision
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				if cluster.Status.BlueGreen.GreenRevision != "" {
					activeRevision = cluster.Status.BlueGreen.GreenRevision
				}
			}
		}
		statefulSetName.Name = fmt.Sprintf("%s-%s", cluster.Name, activeRevision)
	}

	var readyReplicas int32
	var available bool
	err = r.Get(ctx, statefulSetName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get StatefulSet %s/%s for status update: %w", cluster.Namespace, cluster.Name, err)
		}
		// StatefulSet not found - cluster is initializing
		readyReplicas = 0
		available = false
	} else {
		readyReplicas = statefulSet.Status.ReadyReplicas
		desiredReplicas := cluster.Spec.Replicas
		available = readyReplicas == desiredReplicas && readyReplicas > 0

		// Check if StatefulSet controller is still processing (hasn't caught up with spec changes)
		// or if status fields are potentially stale (pods may be ready but status not updated yet)
		statefulSetStillScaling := statefulSet.Status.ObservedGeneration < statefulSet.Generation ||
			statefulSet.Status.Replicas < desiredReplicas

		// Even when StatefulSet controller has caught up, ReadyReplicas might be stale
		// if pods are ready but the status update hasn't propagated yet.
		// If we have the desired number of replicas but ReadyReplicas is less,
		// we should requeue to catch the status update.
		statusPotentiallyStale := statefulSet.Status.Replicas == desiredReplicas &&
			statefulSet.Status.ReadyReplicas < statefulSet.Status.Replicas &&
			statefulSet.Status.ReadyReplicas < desiredReplicas

		logger.Info("StatefulSet status read for ReadyReplicas calculation",
			"statefulSetReadyReplicas", statefulSet.Status.ReadyReplicas,
			"statefulSetReplicas", statefulSet.Status.Replicas,
			"statefulSetCurrentReplicas", statefulSet.Status.CurrentReplicas,
			"statefulSetUpdatedReplicas", statefulSet.Status.UpdatedReplicas,
			"statefulSetObservedGeneration", statefulSet.Status.ObservedGeneration,
			"statefulSetGeneration", statefulSet.Generation,
			"desiredReplicas", desiredReplicas,
			"calculatedReadyReplicas", readyReplicas,
			"available", available,
			"statefulSetStillScaling", statefulSetStillScaling,
			"statusPotentiallyStale", statusPotentiallyStale)

		// If StatefulSet is still scaling or status is potentially stale, set phase and persist status before requeue
		if statefulSetStillScaling || statusPotentiallyStale {
			logger.V(1).Info("StatefulSet status may be stale; setting phase and requeuing to check status",
				"observedGeneration", statefulSet.Status.ObservedGeneration,
				"generation", statefulSet.Generation,
				"currentReplicas", statefulSet.Status.Replicas,
				"readyReplicas", statefulSet.Status.ReadyReplicas,
				"desiredReplicas", desiredReplicas,
				"stillScaling", statefulSetStillScaling,
				"statusStale", statusPotentiallyStale)

			// Set phase and ready replicas before requeuing to ensure status is persisted
			cluster.Status.ReadyReplicas = readyReplicas

			// Update per-cluster metrics for ready replicas
			clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
			clusterMetrics.SetReadyReplicas(readyReplicas)

			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
			if upgradeFailed {
				cluster.Status.Phase = openbaov1alpha1.ClusterPhaseFailed
			} else if upgradeInProgress {
				cluster.Status.Phase = openbaov1alpha1.ClusterPhaseUpgrading
			} else if backupInProgress {
				cluster.Status.Phase = openbaov1alpha1.ClusterPhaseBackingUp
			}

			clusterMetrics.SetPhase(cluster.Status.Phase)

			// Set basic conditions before requeuing to ensure they're persisted
			// TODO: Add constants for these reasons
			now := metav1.Now()
			availableStatus := metav1.ConditionFalse
			availableReason := ReasonNotReady
			availableMessage := fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, cluster.Spec.Replicas)
			if available {
				availableStatus = metav1.ConditionTrue
				availableReason = ReasonAllReplicasReady
				availableMessage = fmt.Sprintf("All %d replicas are ready", readyReplicas)
			} else if readyReplicas == 0 {
				availableStatus = metav1.ConditionFalse
				availableReason = ReasonNoReplicasReady
				availableMessage = "No replicas are ready yet"
			}

			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionAvailable),
				Status:             availableStatus,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             availableReason,
				Message:            availableMessage,
			})

			degradedStatus := metav1.ConditionFalse
			degradedReason := constants.ReasonReconciling
			degradedMessage := "No degradation has been recorded by the controller"

			if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
				degradedStatus = metav1.ConditionTrue
				degradedReason = constants.ReasonBreakGlassRequired
				degradedMessage = "Break glass required; see status.breakGlass for recovery steps"
			} else if cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled && r.AdmissionStatus != nil && !r.AdmissionStatus.SentinelReady {
				degradedStatus = metav1.ConditionTrue
				degradedReason = ReasonAdmissionPoliciesNotReady
				degradedMessage = "Sentinel is enabled but required admission policies are missing or misbound; Sentinel will not be deployed. " + r.AdmissionStatus.SummaryMessage()
			} else if upgradeFailed && cluster.Status.Upgrade != nil {
				degradedStatus = metav1.ConditionTrue
				degradedReason = cluster.Status.Upgrade.LastErrorReason
				degradedMessage = cluster.Status.Upgrade.LastErrorMessage
			} else if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
				degradedStatus = metav1.ConditionTrue
				degradedReason = cluster.Status.Workload.LastError.Reason
				degradedMessage = cluster.Status.Workload.LastError.Message
			} else if cluster.Status.AdminOps != nil && cluster.Status.AdminOps.LastError != nil {
				degradedStatus = metav1.ConditionTrue
				degradedReason = cluster.Status.AdminOps.LastError.Reason
				degradedMessage = cluster.Status.AdminOps.LastError.Message
			} else {
				selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
				if !selfInitEnabled {
					degradedStatus = metav1.ConditionTrue
					degradedReason = ReasonRootTokenStored
					degradedMessage = "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. Anyone with Secret read access in this namespace can access the root token. Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments."
				}
			}

			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionDegraded),
				Status:             degradedStatus,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             degradedReason,
				Message:            degradedMessage,
			})

			upgradeCondStatus := metav1.ConditionFalse
			upgradeCondReason := constants.ReasonIdle
			upgradeCondMessage := "No upgrade is currently in progress"
			if upgradeFailed && cluster.Status.Upgrade != nil {
				upgradeCondStatus = metav1.ConditionFalse
				upgradeCondReason = cluster.Status.Upgrade.LastErrorReason
				upgradeCondMessage = cluster.Status.Upgrade.LastErrorMessage
			} else if upgradeInProgress {
				upgradeCondStatus = metav1.ConditionTrue
				upgradeCondReason = "InProgress"
				if rollingUpgradeInProgress && cluster.Status.Upgrade != nil {
					upgradeCondMessage = fmt.Sprintf("Rolling upgrade from %s to %s (partition=%d)", cluster.Status.Upgrade.FromVersion, cluster.Status.Upgrade.TargetVersion, cluster.Status.Upgrade.CurrentPartition)
				} else if blueGreenInProgress && cluster.Status.BlueGreen != nil {
					upgradeCondMessage = fmt.Sprintf("Blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase)
				} else {
					upgradeCondMessage = "Upgrade in progress"
				}
			}
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionUpgrading),
				Status:             upgradeCondStatus,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             upgradeCondReason,
				Message:            upgradeCondMessage,
			})

			backupCondStatus := metav1.ConditionFalse
			backupCondReason := constants.ReasonIdle
			backupCondMessage := "No backup is currently in progress"
			if backupInProgress {
				backupCondStatus = metav1.ConditionTrue
				backupCondReason = "InProgress"
				if backupJobName != "" {
					backupCondMessage = fmt.Sprintf("Backup Job %s is running", backupJobName)
				} else {
					backupCondMessage = "Backup in progress"
				}
			}
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionBackingUp),
				Status:             backupCondStatus,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             backupCondReason,
				Message:            backupCondMessage,
			})

			// Persist status before requeuing
			if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update status before requeue: %w", err)
			}

			return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
		}
	}

	cluster.Status.ReadyReplicas = readyReplicas

	// Update per-cluster metrics for ready replicas and phase. Metrics are
	// derived from the status view and do not influence reconciliation logic.
	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.SetReadyReplicas(readyReplicas)

	// Set initial CurrentVersion if empty (first reconcile after initialization)
	if cluster.Status.CurrentVersion == "" && cluster.Status.Initialized {
		cluster.Status.CurrentVersion = cluster.Spec.Version
		logger.Info("Set initial CurrentVersion", "version", cluster.Spec.Version)
	}

	cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	if upgradeFailed {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseFailed
	} else if upgradeInProgress {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseUpgrading
	} else if backupInProgress {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseBackingUp
	} else if available {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	}

	clusterMetrics.SetPhase(cluster.Status.Phase)

	now := metav1.Now()

	podSelector := map[string]string{
		constants.LabelAppInstance:  cluster.Name,
		constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	}
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen && activeRevision != "" {
		podSelector[constants.LabelOpenBaoRevision] = activeRevision
	}

	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelector),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	var leaderCount int
	var leaderName string
	var pod0 *corev1.Pod
	pod0Name := fmt.Sprintf("%s-0", cluster.Name)
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen && activeRevision != "" {
		pod0Name = fmt.Sprintf("%s-%s-0", cluster.Name, activeRevision)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Name == pod0Name {
			pod0 = pod
		}

		active, present, err := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelActive)
		if err != nil {
			logger.V(1).Info("Invalid OpenBao active label value", "pod", pod.Name, "error", err)
			continue
		}
		if present && active {
			leaderCount++
			leaderName = pod.Name
		}
	}

	cluster.Status.ActiveLeader = leaderName

	if pod0 != nil {
		initialized, present, err := openbaolabels.ParseBoolLabel(pod0.Labels, openbaolabels.LabelInitialized)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid %q label on pod %s: %w", openbaolabels.LabelInitialized, pod0.Name, err)
		}
		if present {
			status := metav1.ConditionFalse
			reason := "NotInitialized"
			message := "OpenBao reports not initialized"
			if initialized {
				status = metav1.ConditionTrue
				reason = "Initialized"
				message = "OpenBao reports initialized"
			}
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoInitialized),
				Status:             status,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             reason,
				Message:            message,
			})
		} else {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoInitialized),
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             constants.ReasonUnknown,
				Message:            "OpenBao initialization state is not yet available via service registration",
			})
		}

		sealed, present, err := openbaolabels.ParseBoolLabel(pod0.Labels, openbaolabels.LabelSealed)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid %q label on pod %s: %w", openbaolabels.LabelSealed, pod0.Name, err)
		}
		if present {
			status := metav1.ConditionFalse
			reason := "Unsealed"
			message := "OpenBao reports unsealed"
			if sealed {
				status = metav1.ConditionTrue
				reason = "Sealed"
				message = "OpenBao reports sealed"
			}
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoSealed),
				Status:             status,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             reason,
				Message:            message,
			})
		} else {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoSealed),
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             constants.ReasonUnknown,
				Message:            "OpenBao seal state is not yet available via service registration",
			})
		}
	}

	switch leaderCount {
	case 0:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonLeaderUnknown,
			Message:            "No active leader label observed on pods",
		})
	case 1:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonLeaderFound,
			Message:            fmt.Sprintf("Leader is %s", leaderName),
		})
	default:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonMultipleLeaders,
			Message:            fmt.Sprintf("Multiple leaders detected via pod labels (%d)", leaderCount),
		})
	}

	availableStatus := metav1.ConditionFalse
	availableReason := "NotReady"
	availableMessage := fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, cluster.Spec.Replicas)
	if available {
		availableStatus = metav1.ConditionTrue
		availableReason = "AllReplicasReady"
		availableMessage = fmt.Sprintf("All %d replicas are ready", readyReplicas)
	} else if readyReplicas == 0 {
		availableStatus = metav1.ConditionFalse
		availableReason = "NoReplicasReady"
		availableMessage = "No replicas are ready yet"
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             availableStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             availableReason,
		Message:            availableMessage,
	})

	degradedStatus := metav1.ConditionFalse
	degradedReason := constants.ReasonReconciling
	degradedMessage := "No degradation has been recorded by the controller"

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		degradedStatus = metav1.ConditionTrue
		degradedReason = constants.ReasonBreakGlassRequired
		degradedMessage = "Break glass required; see status.breakGlass for recovery steps"
	} else if cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled && r.AdmissionStatus != nil && !r.AdmissionStatus.SentinelReady {
		degradedStatus = metav1.ConditionTrue
		degradedReason = ReasonAdmissionPoliciesNotReady
		degradedMessage = "Sentinel is enabled but required admission policies are missing or misbound; Sentinel will not be deployed. " + r.AdmissionStatus.SummaryMessage()
	} else if upgradeFailed && cluster.Status.Upgrade != nil {
		degradedStatus = metav1.ConditionTrue
		degradedReason = cluster.Status.Upgrade.LastErrorReason
		degradedMessage = cluster.Status.Upgrade.LastErrorMessage
	} else if cluster.Status.Workload != nil && cluster.Status.Workload.LastError != nil {
		degradedStatus = metav1.ConditionTrue
		degradedReason = cluster.Status.Workload.LastError.Reason
		degradedMessage = cluster.Status.Workload.LastError.Message
	} else if cluster.Status.AdminOps != nil && cluster.Status.AdminOps.LastError != nil {
		degradedStatus = metav1.ConditionTrue
		degradedReason = cluster.Status.AdminOps.LastError.Reason
		degradedMessage = cluster.Status.AdminOps.LastError.Message
	} else {
		selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
		if !selfInitEnabled {
			degradedStatus = metav1.ConditionTrue
			degradedReason = ReasonRootTokenStored
			degradedMessage = "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. Anyone with Secret read access in this namespace can access the root token. Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments."
		}
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             degradedStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             degradedReason,
		Message:            degradedMessage,
	})

	upgradeCondStatus := metav1.ConditionFalse
	upgradeCondReason := constants.ReasonIdle
	upgradeCondMessage := "No upgrade is currently in progress"
	if upgradeFailed && cluster.Status.Upgrade != nil {
		upgradeCondStatus = metav1.ConditionFalse
		upgradeCondReason = cluster.Status.Upgrade.LastErrorReason
		upgradeCondMessage = cluster.Status.Upgrade.LastErrorMessage
	} else if upgradeInProgress {
		upgradeCondStatus = metav1.ConditionTrue
		upgradeCondReason = "InProgress"
		if rollingUpgradeInProgress && cluster.Status.Upgrade != nil {
			upgradeCondMessage = fmt.Sprintf("Rolling upgrade from %s to %s (partition=%d)", cluster.Status.Upgrade.FromVersion, cluster.Status.Upgrade.TargetVersion, cluster.Status.Upgrade.CurrentPartition)
		} else if blueGreenInProgress && cluster.Status.BlueGreen != nil {
			upgradeCondMessage = fmt.Sprintf("Blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase)
		} else {
			upgradeCondMessage = "Upgrade in progress"
		}
	}
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             upgradeCondStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             upgradeCondReason,
		Message:            upgradeCondMessage,
	})

	backupCondStatus := metav1.ConditionFalse
	backupCondReason := constants.ReasonIdle
	backupCondMessage := "No backup is currently in progress"
	if backupInProgress {
		backupCondStatus = metav1.ConditionTrue
		backupCondReason = "InProgress"
		if backupJobName != "" {
			backupCondMessage = fmt.Sprintf("Backup Job %s is running", backupJobName)
		} else {
			backupCondMessage = "Backup in progress"
		}
	}
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionBackingUp),
		Status:             backupCondStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             backupCondReason,
		Message:            backupCondMessage,
	})

	// Set etcd encryption warning condition.
	// The operator cannot verify etcd encryption status, but it should warn users
	// that security relies on underlying K8s secret encryption at rest.
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonEtcdEncryptionUnknown,
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

	// Set SecurityRisk condition for Development profile
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionSecurityRisk),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonDevelopmentProfile,
			Message:            "Cluster is using Development profile with relaxed security. Not suitable for production.",
		})
	} else {
		// Remove condition if profile is Hardened or not set
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionSecurityRisk))
	}

	// Set ProductionReady condition based on security posture and bootstrap configuration.
	productionReadyStatus, productionReadyReason, productionReadyMessage := evaluateProductionReady(cluster)
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             productionReadyStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             productionReadyReason,
		Message:            productionReadyMessage,
	})

	// SECURITY: Warn when SelfInit is disabled - the operator will store the root token.
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		logger.Info("SECURITY WARNING: SelfInit is disabled - root token will be stored in Secret",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name,
			"secret_name", cluster.Name+"-root-token")
	}

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster", "readyReplicas", readyReplicas, "phase", cluster.Status.Phase, "currentVersion", cluster.Status.CurrentVersion)

	// Requeue logic to ensure status updates promptly when replicas become ready.
	// Since we don't watch StatefulSets directly (for security reasons), we need to requeue
	// to catch when pods transition to ready state.
	previousReadyReplicas := original.Status.ReadyReplicas
	readyReplicasChanged := readyReplicas != previousReadyReplicas

	if !available && readyReplicas > 0 {
		// Not all replicas are ready yet - requeue to check again
		logger.V(1).Info("Not all replicas are ready; requeuing to check status", "readyReplicas", readyReplicas, "desiredReplicas", cluster.Spec.Replicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	if available && readyReplicasChanged {
		// All replicas just became ready - requeue once to ensure status is persisted and visible
		logger.V(1).Info("All replicas became ready; requeuing once to ensure status is persisted", "readyReplicas", readyReplicas, "previousReadyReplicas", previousReadyReplicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	return ctrl.Result{}, nil
}
