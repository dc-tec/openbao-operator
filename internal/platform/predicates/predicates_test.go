package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func baseClusterForPredicate() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "cluster-a",
			Namespace:   "ns-a",
			Generation:  1,
			Finalizers:  []string{"finalizer.openbao.org/test"},
			Labels:      map[string]string{"app": "openbao"},
			Annotations: map[string]string{"key": "value"},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{FromVersion: "2.4.4", TargetVersion: "2.4.5"},
			Backup:  &openbaov1alpha1.BackupStatus{LastFailureReason: "none"},
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase: openbaov1alpha1.PhaseSyncing,
			},
			OperationLock: &openbaov1alpha1.OperationLockStatus{
				Operation: openbaov1alpha1.ClusterOperationUpgrade,
				Holder:    "holder-a",
				Message:   "in progress",
			},
			BreakGlass: &openbaov1alpha1.BreakGlassStatus{Active: false},
			Workload:   &openbaov1alpha1.WorkloadControllerStatus{LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "X", Message: "Y"}},
			AdminOps:   &openbaov1alpha1.AdminOpsControllerStatus{LastError: &openbaov1alpha1.ControllerErrorStatus{Reason: "A", Message: "B"}},
		},
	}
}

func TestShouldReconcileOpenBaoClusterUpdate_MetadataAndGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(oldC, newC *openbaov1alpha1.OpenBaoCluster)
		want   bool
	}{
		{
			name: "status-only update filtered by default",
			mutate: func(_, newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.ReadyReplicas = 1
			},
			want: false,
		},
		{
			name: "generation change reconciles",
			mutate: func(_, newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Generation++
			},
			want: true,
		},
		{
			name: "deletion timestamp change reconciles",
			mutate: func(_, newC *openbaov1alpha1.OpenBaoCluster) {
				now := metav1.Now()
				newC.DeletionTimestamp = &now
			},
			want: true,
		},
		{
			name: "finalizer change reconciles",
			mutate: func(_, newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Finalizers = append(newC.Finalizers, "other")
			},
			want: true,
		},
		{
			name: "label change reconciles",
			mutate: func(_, newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Labels["new"] = "label"
			},
			want: true,
		},
		{
			name: "annotation change reconciles",
			mutate: func(_, newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Annotations["new"] = "annotation"
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			oldC := baseClusterForPredicate()
			newC := oldC.DeepCopy()
			tt.mutate(oldC, newC)

			got := shouldReconcileOpenBaoClusterUpdate(OpenBaoClusterPredicateOptions{}, oldC, newC)
			if got != tt.want {
				t.Fatalf("shouldReconcileOpenBaoClusterUpdate()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldReconcileOpenBaoClusterUpdate_StatusOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opts   OpenBaoClusterPredicateOptions
		mutate func(newC *openbaov1alpha1.OpenBaoCluster)
		want   bool
	}{
		{
			name: "upgrade status change with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnUpgradeStatus: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.Upgrade = &openbaov1alpha1.UpgradeProgress{FromVersion: "2.4.5", TargetVersion: "2.4.6"}
			},
			want: true,
		},
		{
			name: "backup status change with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnBackupStatus: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "failed"}
			},
			want: true,
		},
		{
			name: "bluegreen status change with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnBlueGreenStatus: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{Phase: openbaov1alpha1.PhaseCleanup}
			},
			want: true,
		},
		{
			name: "breakglass status change with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnBreakGlass: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{Active: true, Nonce: "n1"}
			},
			want: true,
		},
		{
			name: "workload last error with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnWorkloadError: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.Workload.LastError = &openbaov1alpha1.ControllerErrorStatus{Reason: "Changed", Message: "changed"}
			},
			want: true,
		},
		{
			name: "adminops last error with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnAdminOpsError: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.AdminOps.LastError = &openbaov1alpha1.ControllerErrorStatus{Reason: "Changed", Message: "changed"}
			},
			want: true,
		},
		{
			name: "operation lock change with option enabled",
			opts: OpenBaoClusterPredicateOptions{ReconcileOnOperationLock: true},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
					Operation: openbaov1alpha1.ClusterOperationRestore,
					Holder:    "holder-b",
					Message:   "restore running",
				}
			},
			want: true,
		},
		{
			name: "status change not reconciled when option disabled",
			opts: OpenBaoClusterPredicateOptions{},
			mutate: func(newC *openbaov1alpha1.OpenBaoCluster) {
				newC.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "failed"}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			oldC := baseClusterForPredicate()
			newC := oldC.DeepCopy()
			tt.mutate(newC)
			got := shouldReconcileOpenBaoClusterUpdate(tt.opts, oldC, newC)
			if got != tt.want {
				t.Fatalf("shouldReconcileOpenBaoClusterUpdate()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenBaoClusterPredicateWithOptions_UpdateTypeFallback(t *testing.T) {
	t.Parallel()
	pred := OpenBaoClusterPredicateWithOptions(OpenBaoClusterPredicateOptions{})

	if !pred.Update(event.UpdateEvent{ObjectOld: &appsv1.StatefulSet{}, ObjectNew: baseClusterForPredicate()}) {
		t.Fatal("expected reconcile=true when old object type assertion fails")
	}

	if !pred.Update(event.UpdateEvent{ObjectOld: baseClusterForPredicate(), ObjectNew: &appsv1.StatefulSet{}}) {
		t.Fatal("expected reconcile=true when new object type assertion fails")
	}
}

func TestOpenBaoClusterPredicateAndStatefulSetPredicate_AlwaysTrueEvents(t *testing.T) {
	t.Parallel()

	clusterPred := OpenBaoClusterPredicate()
	stsPred := StatefulSetReadyReplicasPredicate()

	if !clusterPred.Create(event.CreateEvent{}) || !clusterPred.Delete(event.DeleteEvent{}) || !clusterPred.Generic(event.GenericEvent{}) {
		t.Fatal("openbaocluster predicate should allow create/delete/generic events")
	}

	if !stsPred.Create(event.CreateEvent{}) || !stsPred.Delete(event.DeleteEvent{}) || !stsPred.Generic(event.GenericEvent{}) {
		t.Fatal("statefulset predicate should allow create/delete/generic events")
	}
}

func TestStatefulSetReadyReplicasPredicate_Update(t *testing.T) {
	t.Parallel()
	pred := StatefulSetReadyReplicasPredicate()

	oldSts := &appsv1.StatefulSet{}
	newSts := oldSts.DeepCopy()
	newSts.Status.ReadyReplicas = 1

	if !pred.Update(event.UpdateEvent{ObjectOld: oldSts, ObjectNew: newSts}) {
		t.Fatal("expected reconcile when ReadyReplicas changes")
	}

	oldSts = &appsv1.StatefulSet{}
	newSts = oldSts.DeepCopy()
	if pred.Update(event.UpdateEvent{ObjectOld: oldSts, ObjectNew: newSts}) {
		t.Fatal("expected no reconcile when ReadyReplicas unchanged")
	}

	if !pred.Update(event.UpdateEvent{ObjectOld: baseClusterForPredicate(), ObjectNew: newSts}) {
		t.Fatal("expected reconcile=true on old type assertion failure")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: oldSts, ObjectNew: baseClusterForPredicate()}) {
		t.Fatal("expected reconcile=true on new type assertion failure")
	}
}

func baseTenantForPredicate() *openbaov1alpha1.OpenBaoTenant {
	return &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "tenant-a",
			Namespace:   "tenant-ns",
			Generation:  1,
			Finalizers:  []string{"finalizer.openbao.org/tenant"},
			Labels:      map[string]string{"app": "tenant"},
			Annotations: map[string]string{"anno": "1"},
		},
	}
}

func TestOpenBaoTenantPredicate_Update(t *testing.T) {
	t.Parallel()
	pred := OpenBaoTenantPredicate()

	tests := []struct {
		name   string
		mutate func(*openbaov1alpha1.OpenBaoTenant)
		want   bool
	}{
		{
			name: "generation changed",
			mutate: func(tn *openbaov1alpha1.OpenBaoTenant) {
				tn.Generation++
			},
			want: true,
		},
		{
			name: "deletion timestamp changed",
			mutate: func(tn *openbaov1alpha1.OpenBaoTenant) {
				now := metav1.Now()
				tn.DeletionTimestamp = &now
			},
			want: true,
		},
		{
			name: "finalizers changed",
			mutate: func(tn *openbaov1alpha1.OpenBaoTenant) {
				tn.Finalizers = append(tn.Finalizers, "extra")
			},
			want: true,
		},
		{
			name: "labels changed",
			mutate: func(tn *openbaov1alpha1.OpenBaoTenant) {
				tn.Labels["k"] = "v"
			},
			want: true,
		},
		{
			name: "annotations changed",
			mutate: func(tn *openbaov1alpha1.OpenBaoTenant) {
				tn.Annotations["k"] = "v"
			},
			want: true,
		},
		{
			name: "status only filtered",
			mutate: func(tn *openbaov1alpha1.OpenBaoTenant) {
				tn.Status.Provisioned = true
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			oldTenant := baseTenantForPredicate()
			newTenant := oldTenant.DeepCopy()
			tt.mutate(newTenant)
			if got := pred.Update(event.UpdateEvent{ObjectOld: oldTenant, ObjectNew: newTenant}); got != tt.want {
				t.Fatalf("update=%v, want %v", got, tt.want)
			}
		})
	}

	if !pred.Update(event.UpdateEvent{ObjectOld: &appsv1.StatefulSet{}, ObjectNew: baseTenantForPredicate()}) {
		t.Fatal("expected reconcile=true on old type assertion failure")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: baseTenantForPredicate(), ObjectNew: &appsv1.StatefulSet{}}) {
		t.Fatal("expected reconcile=true on new type assertion failure")
	}
}

func TestResourceGenerationChangedPredicate(t *testing.T) {
	t.Parallel()
	pred := ResourceGenerationChangedPredicate()

	if !pred.Create(event.CreateEvent{}) || !pred.Delete(event.DeleteEvent{}) || !pred.Generic(event.GenericEvent{}) {
		t.Fatal("resource generation predicate should allow create/delete/generic events")
	}

	oldObj := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	newObj := oldObj.DeepCopy()
	newObj.Generation = 2
	if !pred.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}) {
		t.Fatal("expected reconcile when generation changes")
	}

	newObj.Generation = 1
	if pred.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}) {
		t.Fatal("expected no reconcile when generation unchanged")
	}

	if !pred.Update(event.UpdateEvent{ObjectOld: &appsv1.StatefulSet{}, ObjectNew: newObj}) {
		t.Fatal("expected reconcile=true when old object is not metav1.Object")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: &appsv1.StatefulSet{}}) {
		t.Fatal("expected reconcile=true when new object is not metav1.Object")
	}
}
