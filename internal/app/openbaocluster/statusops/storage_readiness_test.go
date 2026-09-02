package statusops

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestEvaluateACMECacheReadiness(t *testing.T) {
	t.Parallel()

	cacheKey := types.NamespacedName{Namespace: "default", Name: "shared-cache"}
	readFailure := errors.New("injected read failure")
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		objects    map[types.NamespacedName]*corev1.PersistentVolumeClaim
		getErrors  map[types.NamespacedName]error
		want       ConditionResult
		applicable bool
		wantGets   []types.NamespacedName
	}{
		{
			name:       "not applicable outside ACME mode",
			cluster:    newStorageReadinessTestCluster(),
			applicable: false,
		},
		{
			name: "not applicable to a single replica without a configured cache",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECacheReadinessTestCluster()
				cluster.Spec.Replicas = 1
				return cluster
			}(),
			applicable: false,
		},
		{
			name:    "required cache claim is unresolved",
			cluster: newACMECacheReadinessTestCluster(),
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonACMECacheNotConfigured,
				Message: "ACME shared cache is required for this topology; configure spec.tls.acme.sharedCache with a RWX PVC",
			},
			applicable: true,
		},
		{
			name:    "configured cache PVC is missing",
			cluster: newExistingACMECacheReadinessTestCluster(cacheKey.Name),
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonACMECacheMissing,
				Message: "ACME shared cache PVC default/shared-cache was not found",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
		{
			name:      "configured cache PVC read fails",
			cluster:   newExistingACMECacheReadinessTestCluster(cacheKey.Name),
			getErrors: map[types.NamespacedName]error{cacheKey: readFailure},
			want: ConditionResult{
				Status:  metav1.ConditionUnknown,
				Reason:  reasonUnknown,
				Message: "Failed to read ACME shared cache PVC default/shared-cache: injected read failure",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
		{
			name:    "configured cache PVC does not support ReadWriteMany",
			cluster: newExistingACMECacheReadinessTestCluster(cacheKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				cacheKey: newStorageReadinessPVC(cacheKey, corev1.ReadWriteOnce, corev1.ClaimPending),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonACMECacheInvalidAccessMode,
				Message: "ACME shared cache PVC default/shared-cache must support ReadWriteMany",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
		{
			name: "configured cache applies when the topology does not require it",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newExistingACMECacheReadinessTestCluster(cacheKey.Name)
				cluster.Spec.Replicas = 1
				return cluster
			}(),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				cacheKey: newStorageReadinessPVC(cacheKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonACMECacheReady,
				Message: "ACME shared cache PVC default/shared-cache is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
		{
			name:    "configured cache PVC is pending",
			cluster: newExistingACMECacheReadinessTestCluster(cacheKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				cacheKey: newStorageReadinessPVC(cacheKey, corev1.ReadWriteMany, corev1.ClaimPending),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonACMECachePending,
				Message: "ACME shared cache PVC default/shared-cache is not Bound yet (phase=Pending)",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
		{
			name:    "configured cache PVC is ready",
			cluster: newExistingACMECacheReadinessTestCluster(cacheKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				cacheKey: newStorageReadinessPVC(cacheKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonACMECacheReady,
				Message: "ACME shared cache PVC default/shared-cache is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
		{
			name: "managed cache resolves the generated PVC name",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECacheReadinessTestCluster()
				cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
					Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
					Size: "1Gi",
				}
				return cluster
			}(),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				{Namespace: "default", Name: "example-acme-cache"}: newStorageReadinessPVC(
					types.NamespacedName{Namespace: "default", Name: "example-acme-cache"},
					corev1.ReadWriteMany,
					corev1.ClaimBound,
				),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonACMECacheReady,
				Message: "ACME shared cache PVC default/example-acme-cache is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantGets: []types.NamespacedName{
				{Namespace: "default", Name: "example-acme-cache"},
			},
		},
		{
			name:    "existing cache trims the configured PVC name",
			cluster: newExistingACMECacheReadinessTestCluster("  shared-cache  "),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				cacheKey: newStorageReadinessPVC(cacheKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonACMECacheReady,
				Message: "ACME shared cache PVC default/shared-cache is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantGets:   []types.NamespacedName{cacheKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterBefore := tt.cluster.DeepCopy()
			reader := &storageReadinessReader{objects: tt.objects, getErrors: tt.getErrors}
			got, applicable := EvaluateACMECacheReadiness(t.Context(), reader, tt.cluster)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EvaluateACMECacheReadiness() result = %#v, want %#v", got, tt.want)
			}
			if applicable != tt.applicable {
				t.Fatalf("EvaluateACMECacheReadiness() applicable = %t, want %t", applicable, tt.applicable)
			}
			if !reflect.DeepEqual(reader.gets, tt.wantGets) {
				t.Fatalf("PVC reads = %#v, want %#v", reader.gets, tt.wantGets)
			}
			if len(reader.listCalls) != 0 {
				t.Fatalf("StatefulSet list calls = %#v, want none", reader.listCalls)
			}
			if !reflect.DeepEqual(tt.cluster, clusterBefore) {
				t.Fatalf("EvaluateACMECacheReadiness() mutated cluster: got %#v, want %#v", tt.cluster, clusterBefore)
			}
		})
	}
}

func TestEvaluateAuditFileStorageReadiness(t *testing.T) {
	t.Parallel()

	claimKey := types.NamespacedName{Namespace: "default", Name: "audit-claim"}
	readFailure := errors.New("injected read failure")
	listFailure := errors.New("injected list failure")
	tests := []struct {
		name         string
		cluster      *openbaov1alpha1.OpenBaoCluster
		objects      map[types.NamespacedName]*corev1.PersistentVolumeClaim
		getErrors    map[types.NamespacedName]error
		statefulSets []appsv1.StatefulSet
		listError    error
		want         ConditionResult
		applicable   bool
		wantOps      []string
	}{
		{
			name:       "not applicable without audit file storage",
			cluster:    newStorageReadinessTestCluster(),
			applicable: false,
		},
		{
			name:    "configured claim is unresolved",
			cluster: newAuditFileStorageReadinessTestCluster(""),
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonAuditFileStorageMissing,
				Message: "Audit file storage is configured but no PVC claim name could be resolved",
			},
			applicable: true,
		},
		{
			name:    "configured PVC is missing",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonAuditFileStorageMissing,
				Message: "Audit file storage PVC default/audit-claim was not found",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim"},
		},
		{
			name:      "configured PVC read fails",
			cluster:   newAuditFileStorageReadinessTestCluster(claimKey.Name),
			getErrors: map[types.NamespacedName]error{claimKey: readFailure},
			want: ConditionResult{
				Status:  metav1.ConditionUnknown,
				Reason:  reasonUnknown,
				Message: "Failed to read audit file storage PVC default/audit-claim: injected read failure",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim"},
		},
		{
			name:    "access mode is checked before phase",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteOnce, corev1.ClaimPending),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonAuditFileStorageInvalidAccessMode,
				Message: "Audit file storage PVC default/audit-claim must support ReadWriteMany",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim"},
		},
		{
			name:    "configured PVC is pending",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimPending),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonAuditFileStoragePending,
				Message: "Audit file storage PVC default/audit-claim is not Bound yet (phase=Pending)",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim"},
		},
		{
			name:    "StatefulSet list fails after the PVC read",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			listError: listFailure,
			want: ConditionResult{
				Status:  metav1.ConditionUnknown,
				Reason:  reasonUnknown,
				Message: "Failed to inspect OpenBao StatefulSets for audit file storage mounts: injected list failure",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name:    "ready before a StatefulSet exists",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonAuditFileStorageReady,
				Message: "Audit file storage PVC default/audit-claim is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name: "managed storage resolves the generated PVC name",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newStorageReadinessTestCluster()
				cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
					Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
					Size: "5Gi",
				}
				return cluster
			}(),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				{Namespace: "default", Name: "example-audit"}: newStorageReadinessPVC(
					types.NamespacedName{Namespace: "default", Name: "example-audit"},
					corev1.ReadWriteMany,
					corev1.ClaimBound,
				),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonAuditFileStorageReady,
				Message: "Audit file storage PVC default/example-audit is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantOps:    []string{"get default/example-audit", "list statefulsets"},
		},
		{
			name:    "existing storage trims the configured PVC name",
			cluster: newAuditFileStorageReadinessTestCluster("  audit-claim  "),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonAuditFileStorageReady,
				Message: "Audit file storage PVC default/audit-claim is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name:    "StatefulSet is missing the requested volume",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			statefulSets: []appsv1.StatefulSet{
				*newAuditFileStorageReadinessStatefulSet("example", claimKey.Name, auditMountNoVolume),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  constants.ReasonAuditFileStorageStatefulSetRecreateRequired,
				Message: "StatefulSet default/example is missing the requested audit file storage volume or mount; recreate the StatefulSet or create a new workload revision so locked pod-template fields can be applied",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name:    "StatefulSet is missing the Bao container",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			statefulSets: []appsv1.StatefulSet{
				*newAuditFileStorageReadinessStatefulSet("example", claimKey.Name, auditMountNoBaoContainer),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  constants.ReasonAuditFileStorageStatefulSetRecreateRequired,
				Message: "StatefulSet default/example is missing the requested audit file storage volume or mount; recreate the StatefulSet or create a new workload revision so locked pod-template fields can be applied",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name:    "StatefulSet has the wrong Bao mount",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			statefulSets: []appsv1.StatefulSet{
				*newAuditFileStorageReadinessStatefulSet("example", claimKey.Name, auditMountWrongPath),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  constants.ReasonAuditFileStorageStatefulSetRecreateRequired,
				Message: "StatefulSet default/example is missing the requested audit file storage volume or mount; recreate the StatefulSet or create a new workload revision so locked pod-template fields can be applied",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name:    "first incompatible StatefulSet is reported",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			statefulSets: []appsv1.StatefulSet{
				*newAuditFileStorageReadinessStatefulSet("compatible", claimKey.Name, auditMountValid),
				*newAuditFileStorageReadinessStatefulSet("first-incompatible", claimKey.Name, auditMountWrongSubPath),
				*newAuditFileStorageReadinessStatefulSet("second-incompatible", claimKey.Name, auditMountWrongPath),
			},
			want: ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  constants.ReasonAuditFileStorageStatefulSetRecreateRequired,
				Message: "StatefulSet default/first-incompatible is missing the requested audit file storage volume or mount; recreate the StatefulSet or create a new workload revision so locked pod-template fields can be applied",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
		{
			name:    "all StatefulSets have the requested mount",
			cluster: newAuditFileStorageReadinessTestCluster(claimKey.Name),
			objects: map[types.NamespacedName]*corev1.PersistentVolumeClaim{
				claimKey: newStorageReadinessPVC(claimKey, corev1.ReadWriteMany, corev1.ClaimBound),
			},
			statefulSets: []appsv1.StatefulSet{
				*newAuditFileStorageReadinessStatefulSet("example", claimKey.Name, auditMountValid),
				*newAuditFileStorageReadinessStatefulSet("example-read", claimKey.Name, auditMountValid),
			},
			want: ConditionResult{
				Status:  metav1.ConditionTrue,
				Reason:  reasonAuditFileStorageReady,
				Message: "Audit file storage PVC default/audit-claim is Bound with ReadWriteMany access",
			},
			applicable: true,
			wantOps:    []string{"get default/audit-claim", "list statefulsets"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clusterBefore := tt.cluster.DeepCopy()
			reader := &storageReadinessReader{
				objects:      tt.objects,
				getErrors:    tt.getErrors,
				statefulSets: tt.statefulSets,
				listError:    tt.listError,
			}
			got, applicable := EvaluateAuditFileStorageReadiness(t.Context(), reader, tt.cluster)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EvaluateAuditFileStorageReadiness() result = %#v, want %#v", got, tt.want)
			}
			if applicable != tt.applicable {
				t.Fatalf("EvaluateAuditFileStorageReadiness() applicable = %t, want %t", applicable, tt.applicable)
			}
			if !reflect.DeepEqual(reader.operations, tt.wantOps) {
				t.Fatalf("reader operations = %#v, want %#v", reader.operations, tt.wantOps)
			}
			assertStorageReadinessListOptions(t, reader.listCalls, tt.cluster, len(tt.wantOps) == 2)
			if !reflect.DeepEqual(tt.cluster, clusterBefore) {
				t.Fatalf("EvaluateAuditFileStorageReadiness() mutated cluster: got %#v, want %#v", tt.cluster, clusterBefore)
			}
		})
	}
}

func TestAuditFileStorageStatefulSetHelpers(t *testing.T) {
	t.Parallel()

	cluster := newAuditFileStorageReadinessTestCluster("audit-claim")
	valid := newAuditFileStorageReadinessStatefulSet("example", "audit-claim", auditMountValid)
	tests := []struct {
		name        string
		statefulSet *appsv1.StatefulSet
		mutate      func(*appsv1.StatefulSet)
		want        bool
	}{
		{name: "nil StatefulSet is treated as conforming", want: true},
		{name: "valid volume and mount", statefulSet: valid, want: true},
		{
			name:        "wrong claim",
			statefulSet: valid,
			mutate: func(statefulSet *appsv1.StatefulSet) {
				statefulSet.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = storageReadinessWrongValue
			},
		},
		{
			name:        "nil PVC source",
			statefulSet: valid,
			mutate: func(statefulSet *appsv1.StatefulSet) {
				statefulSet.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim = nil
			},
		},
		{
			name:        "wrong volume name",
			statefulSet: valid,
			mutate: func(statefulSet *appsv1.StatefulSet) {
				statefulSet.Spec.Template.Spec.Volumes[0].Name = storageReadinessWrongValue
			},
		},
		{
			name:        "wrong mount name",
			statefulSet: valid,
			mutate: func(statefulSet *appsv1.StatefulSet) {
				statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name = storageReadinessWrongValue
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			statefulSet := tt.statefulSet
			if statefulSet != nil {
				statefulSet = statefulSet.DeepCopy()
				if tt.mutate != nil {
					tt.mutate(statefulSet)
				}
			}
			if got := statefulSetHasAuditFileStorageMount(statefulSet, cluster); got != tt.want {
				t.Fatalf("statefulSetHasAuditFileStorageMount() = %t, want %t", got, tt.want)
			}
		})
	}

	name, recreateRequired, err := auditFileStorageStatefulSetRecreateRequired(t.Context(), nil, cluster)
	if err != nil || name != "" || recreateRequired {
		t.Fatalf(
			"auditFileStorageStatefulSetRecreateRequired(nil reader) = (%q, %t, %v), want (\"\", false, nil)",
			name,
			recreateRequired,
			err,
		)
	}
}

type storageReadinessReader struct {
	objects      map[types.NamespacedName]*corev1.PersistentVolumeClaim
	getErrors    map[types.NamespacedName]error
	statefulSets []appsv1.StatefulSet
	listError    error

	gets       []types.NamespacedName
	listCalls  []client.ListOptions
	operations []string
}

func (r *storageReadinessReader) Get(
	_ context.Context,
	key client.ObjectKey,
	obj client.Object,
	_ ...client.GetOption,
) error {
	r.gets = append(r.gets, key)
	r.operations = append(r.operations, "get "+key.String())
	if err := r.getErrors[key]; err != nil {
		return err
	}
	pvc, ok := r.objects[key]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "persistentvolumeclaims"}, key.Name)
	}
	destination, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return errors.New("storage readiness reader only supports PersistentVolumeClaim reads")
	}
	*destination = *pvc.DeepCopy()
	return nil
}

func (r *storageReadinessReader) List(
	_ context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	r.operations = append(r.operations, "list statefulsets")
	listOptions := *(&client.ListOptions{}).ApplyOptions(opts)
	r.listCalls = append(r.listCalls, listOptions)
	if r.listError != nil {
		return r.listError
	}
	destination, ok := list.(*appsv1.StatefulSetList)
	if !ok {
		return errors.New("storage readiness reader only supports StatefulSet lists")
	}
	destination.Items = make([]appsv1.StatefulSet, len(r.statefulSets))
	for i := range r.statefulSets {
		r.statefulSets[i].DeepCopyInto(&destination.Items[i])
	}
	return nil
}

func assertStorageReadinessListOptions(
	t *testing.T,
	listCalls []client.ListOptions,
	cluster *openbaov1alpha1.OpenBaoCluster,
	wantList bool,
) {
	t.Helper()
	if !wantList {
		if len(listCalls) != 0 {
			t.Fatalf("StatefulSet list calls = %#v, want none", listCalls)
		}
		return
	}
	if len(listCalls) != 1 {
		t.Fatalf("StatefulSet list calls = %#v, want one", listCalls)
	}
	if listCalls[0].Namespace != cluster.Namespace {
		t.Fatalf("StatefulSet list namespace = %q, want %q", listCalls[0].Namespace, cluster.Namespace)
	}
	wantSelector := labels.SelectorFromSet(resourceidentity.Labels(cluster)).String()
	if listCalls[0].LabelSelector == nil || listCalls[0].LabelSelector.String() != wantSelector {
		t.Fatalf(
			"StatefulSet label selector = %v, want %q",
			listCalls[0].LabelSelector,
			wantSelector,
		)
	}
}

type auditMountState int

const (
	storageReadinessWrongValue = "wrong"

	auditMountValid auditMountState = iota
	auditMountNoVolume
	auditMountNoBaoContainer
	auditMountWrongPath
	auditMountWrongSubPath
)

func newStorageReadinessTestCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "example",
			Namespace:       "default",
			Generation:      4,
			ResourceVersion: "3",
			Labels:          map[string]string{"preserve": "label"},
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 1,
			Profile:  openbaov1alpha1.ProfileHardened,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase:              openbaov1alpha1.ClusterPhaseRunning,
			ObservedGeneration: 3,
		},
	}
}

func newACMECacheReadinessTestCluster() *openbaov1alpha1.OpenBaoCluster {
	cluster := newStorageReadinessTestCluster()
	cluster.Spec.Replicas = 3
	cluster.Spec.TLS = openbaov1alpha1.TLSConfig{
		Enabled: true,
		Mode:    openbaov1alpha1.TLSModeACME,
		ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
	}
	return cluster
}

func newExistingACMECacheReadinessTestCluster(claimName string) *openbaov1alpha1.OpenBaoCluster {
	cluster := newACMECacheReadinessTestCluster()
	cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
		Mode:              openbaov1alpha1.ACMESharedCacheModeExistingPVC,
		ExistingClaimName: claimName,
	}
	return cluster
}

func newAuditFileStorageReadinessTestCluster(claimName string) *openbaov1alpha1.OpenBaoCluster {
	cluster := newStorageReadinessTestCluster()
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode:              openbaov1alpha1.AuditFileStorageModeExistingPVC,
		ExistingClaimName: claimName,
	}
	return cluster
}

func newStorageReadinessPVC(
	key types.NamespacedName,
	accessMode corev1.PersistentVolumeAccessMode,
	phase corev1.PersistentVolumeClaimPhase,
) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: phase},
	}
}

func newAuditFileStorageReadinessStatefulSet(
	name string,
	claimName string,
	mountState auditMountState,
) *appsv1.StatefulSet {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name: constants.VolumeAuditFileStorage,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: claimName,
							},
						},
					}},
					Containers: []corev1.Container{{
						Name: constants.ContainerBao,
						VolumeMounts: []corev1.VolumeMount{{
							Name:        constants.VolumeAuditFileStorage,
							MountPath:   portopenbao.AuditFileStorageDefaultMountPath,
							SubPathExpr: portopenbao.AuditFileStoragePodSubPathExpr,
						}},
					}},
				},
			},
		},
	}

	switch mountState {
	case auditMountValid:
	case auditMountNoVolume:
		statefulSet.Spec.Template.Spec.Volumes = nil
	case auditMountNoBaoContainer:
		statefulSet.Spec.Template.Spec.Containers[0].Name = "sidecar"
	case auditMountWrongPath:
		statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath = "/wrong"
	case auditMountWrongSubPath:
		statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[0].SubPathExpr = storageReadinessWrongValue
	}
	return statefulSet
}
