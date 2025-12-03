/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	backupmanager "github.com/openbao/operator/internal/backup"
	certmanager "github.com/openbao/operator/internal/certs"
	inframanager "github.com/openbao/operator/internal/infra"
	initmanager "github.com/openbao/operator/internal/init"
	upgrademanager "github.com/openbao/operator/internal/upgrade"
)

const openBaoClusterFinalizer = "openbao.org/openbaocluster-finalizer"

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	TLSReload   certmanager.ReloadSignaler
	InitManager *initmanager.Manager
}

// +kubebuilder:rbac:groups=openbao.org,resources=openbaoclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openbao.org,resources=openbaoclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openbao.org,resources=openbaoclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch

// Reconcile is part of the main Kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *OpenBaoClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	baseLogger := log.FromContext(ctx)
	logger := baseLogger.WithValues(
		"cluster_namespace", req.Namespace,
		"cluster_name", req.Name,
		"controller", "openbaocluster",
		"reconcile_id", time.Now().UnixNano(),
	)

	logger.Info("Reconciling OpenBaoCluster")

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("OpenBaoCluster resource not found; assuming it was deleted")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoCluster %s/%s: %w", req.Namespace, req.Name, err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		logger.Info("OpenBaoCluster is marked for deletion")
		if containsFinalizer(cluster.Finalizers, openBaoClusterFinalizer) {
			if err := r.handleDeletion(ctx, logger, cluster); err != nil {
				return ctrl.Result{}, err
			}

			cluster.Finalizers = removeFinalizer(cluster.Finalizers, openBaoClusterFinalizer)
			if err := r.Update(ctx, cluster); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove finalizer from OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
			}
		}

		return ctrl.Result{}, nil
	}

	if !containsFinalizer(cluster.Finalizers, openBaoClusterFinalizer) {
		cluster.Finalizers = append(cluster.Finalizers, openBaoClusterFinalizer)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer to OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}

		// Requeue to observe the resource with the finalizer attached.
		return ctrl.Result{}, nil
	}

	if cluster.Spec.Paused {
		logger.Info("Reconciliation is paused for OpenBaoCluster")
		if err := r.updateStatusForPaused(ctx, logger, cluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcileCerts(ctx, logger, cluster); err != nil {
		now := metav1.Now()
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Error",
			Message:            fmt.Sprintf("failed to reconcile TLS assets: %v", err),
		})

		if statusErr := r.Status().Update(ctx, cluster); statusErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update TLSReady condition for OpenBaoCluster %s/%s after TLS error: %w", cluster.Namespace, cluster.Name, statusErr)
		}

		return ctrl.Result{}, err
	}

	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Ready",
		Message:            "TLS assets are provisioned",
	})

	if err := r.reconcileInfra(ctx, logger, cluster); err != nil {
		return ctrl.Result{}, err
	}

	initResult, err := r.reconcileInit(ctx, logger, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileUpgrade(ctx, logger, cluster); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileBackup(ctx, logger, cluster); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, logger, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// If initialization is still in progress, requeue so that the init manager
	// can retry once the first pod is running and the API is reachable.
	if initResult.RequeueAfter > 0 {
		return initResult, nil
	}

	return ctrl.Result{}, nil
}

func (r *OpenBaoClusterReconciler) reconcileCerts(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling TLS assets for OpenBaoCluster")
	manager := certmanager.NewManagerWithReloader(r.Client, r.Scheme, r.TLSReload)
	return manager.Reconcile(ctx, logger, cluster)
}

func (r *OpenBaoClusterReconciler) reconcileInfra(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling infrastructure for OpenBaoCluster")
	manager := inframanager.NewManager(r.Client, r.Scheme)
	if err := manager.Reconcile(ctx, logger, cluster); err != nil {
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			now := metav1.Now()
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionDegraded),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             "GatewayAPIMissing",
				Message:            "Gateway API CRDs are not installed but spec.gateway.enabled is true; install Gateway API CRDs or disable spec.gateway to clear this condition.",
			})

			if statusErr := r.Status().Update(ctx, cluster); statusErr != nil {
				return fmt.Errorf("failed to update Degraded condition for OpenBaoCluster %s/%s after Gateway API error: %w", cluster.Namespace, cluster.Name, statusErr)
			}

			return nil
		}
		return err
	}

	return nil
}

func (r *OpenBaoClusterReconciler) reconcileInit(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	if r.InitManager == nil {
		// InitManager is optional - if not set, skip initialization check
		return ctrl.Result{}, nil
	}

	// Skip initialization if cluster is already initialized
	if cluster.Status.Initialized {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling initialization for OpenBaoCluster")
	if err := r.InitManager.Reconcile(ctx, logger, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// If initialization is still in progress, requeue so that the init manager
	// can retry once the first pod is running and the API is reachable.
	if !cluster.Status.Initialized {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OpenBaoClusterReconciler) reconcileUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling upgrades for OpenBaoCluster")
	manager := upgrademanager.NewManager(r.Client)
	return manager.Reconcile(ctx, logger, cluster)
}

func (r *OpenBaoClusterReconciler) reconcileBackup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling backups for OpenBaoCluster")
	manager := backupmanager.NewManager(r.Client, r.Scheme)
	return manager.Reconcile(ctx, logger, cluster)
}

func (r *OpenBaoClusterReconciler) handleDeletion(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	policy := cluster.Spec.DeletionPolicy
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger.Info("Applying DeletionPolicy for OpenBaoCluster", "deletionPolicy", string(policy))

	infra := inframanager.NewManager(r.Client, r.Scheme)
	if err := infra.Cleanup(ctx, logger, cluster, policy); err != nil {
		return err
	}

	// Backup deletion for DeletionPolicyDeleteAll will be implemented alongside the BackupManager
	// data path. For now, backups are left untouched even when DeleteAll is specified.

	return nil
}

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
		Reason:             "Paused",
		Message:            "Reconciliation is paused; availability is not being evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Paused",
		Message:            "Cluster is paused; no new degradation has been evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Paused",
		Message:            "TLS readiness is not being evaluated while reconciliation is paused",
	})

	if err := r.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Fetch the StatefulSet to observe actual ready replicas
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	var readyReplicas int32
	var available bool
	err := r.Get(ctx, statefulSetName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get StatefulSet %s/%s for status update: %w", cluster.Namespace, cluster.Name, err)
		}
		// StatefulSet not found - cluster is initializing
		readyReplicas = 0
		available = false
	} else {
		readyReplicas = statefulSet.Status.ReadyReplicas
		desiredReplicas := cluster.Spec.Replicas
		available = readyReplicas == desiredReplicas && readyReplicas > 0
	}

	cluster.Status.ReadyReplicas = readyReplicas

	// Set initial CurrentVersion if empty (first reconcile after initialization)
	if cluster.Status.CurrentVersion == "" && cluster.Status.Initialized {
		cluster.Status.CurrentVersion = cluster.Spec.Version
		logger.Info("Set initial CurrentVersion", "version", cluster.Spec.Version)
	}

	// Only update phase if no upgrade is in progress
	// The upgrade manager controls phase during upgrades
	if cluster.Status.Upgrade == nil {
		if cluster.Status.Phase == "" {
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
		}

		// Update phase based on ready replicas (when not upgrading)
		if readyReplicas == 0 {
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
		} else if available {
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
		} else {
			// Some replicas are ready but not all
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
		}
	}

	now := metav1.Now()

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

	// Only set Degraded=False if not already set by another component (e.g., upgrade manager)
	degradedCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
	if degradedCond == nil || (degradedCond.Status != metav1.ConditionTrue) {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Reconciling",
			Message:            "No degradation has been recorded by the controller",
		})
	}

	// Only set Upgrading=False if no upgrade is in progress
	// The upgrade manager manages this condition during upgrades
	if cluster.Status.Upgrade == nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionUpgrading),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Idle",
			Message:            "No upgrade is currently in progress",
		})
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionBackingUp),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Idle",
		Message:            "No backup is currently in progress",
	})

	// Set etcd encryption warning condition.
	// The operator cannot verify etcd encryption status, but it should warn users
	// that security relies on underlying K8s secret encryption at rest.
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "EtcdEncryptionUnknown",
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

	if err := r.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster", "readyReplicas", readyReplicas, "phase", cluster.Status.Phase, "currentVersion", cluster.Status.CurrentVersion)

	return nil
}

func containsFinalizer(finalizers []string, value string) bool {
	for _, f := range finalizers {
		if f == value {
			return true
		}
	}
	return false
}

func removeFinalizer(finalizers []string, value string) []string {
	result := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f == value {
			continue
		}
		result = append(result, f)
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
// It registers watches on the OpenBaoCluster CR and all owned resources (StatefulSet,
// Services, ConfigMaps, Secrets, ServiceAccounts, and Ingresses) so that changes
// to child resources trigger reconciliation of the parent OpenBaoCluster.
func (r *OpenBaoClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{}).
		// Watch owned StatefulSets - triggers reconcile when StatefulSet changes
		Owns(&appsv1.StatefulSet{}).
		// Watch owned Services - triggers reconcile when Service changes
		Owns(&corev1.Service{}).
		// Watch owned ConfigMaps - triggers reconcile when ConfigMap changes
		Owns(&corev1.ConfigMap{}).
		// Watch owned Secrets - triggers reconcile when Secret changes
		Owns(&corev1.Secret{}).
		// Watch owned ServiceAccounts - triggers reconcile when ServiceAccount changes
		Owns(&corev1.ServiceAccount{}).
		// Watch owned Ingresses - triggers reconcile when Ingress changes
		Owns(&networkingv1.Ingress{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
		}).
		Named("openbaocluster").
		Complete(r)
}
