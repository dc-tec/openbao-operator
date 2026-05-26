package openbaocluster

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestSetAuditFileStorageReadyCondition(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

	newCluster := func() *openbaov1alpha1.OpenBaoCluster {
		return &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{
					Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
					Size: "5Gi",
				},
			},
		}
	}

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		objects    []client.Object
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "configured storage missing pvc",
			cluster:    newCluster(),
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonAuditFileStorageMissing,
		},
		{
			name:    "configured storage pending pvc",
			cluster: newCluster(),
			objects: []client.Object{
				auditFileStoragePVC(corev1.ReadWriteMany, corev1.ClaimPending),
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonAuditFileStoragePending,
		},
		{
			name:    "configured storage invalid access mode",
			cluster: newCluster(),
			objects: []client.Object{
				auditFileStoragePVC(corev1.ReadWriteOnce, corev1.ClaimBound),
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonAuditFileStorageInvalidAccessMode,
		},
		{
			name:    "configured storage ready before statefulset exists",
			cluster: newCluster(),
			objects: []client.Object{
				auditFileStoragePVC(corev1.ReadWriteMany, corev1.ClaimBound),
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonAuditFileStorageReady,
		},
		{
			name:    "existing statefulset requires recreate",
			cluster: newCluster(),
			objects: []client.Object{
				auditFileStoragePVC(corev1.ReadWriteMany, corev1.ClaimBound),
				openBaoStatefulSetWithoutAuditStorage(newCluster()),
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonAuditFileStorageStatefulSetRecreateRequired,
		},
		{
			name:    "existing statefulset already mounted",
			cluster: newCluster(),
			objects: []client.Object{
				auditFileStoragePVC(corev1.ReadWriteMany, corev1.ClaimBound),
				openBaoStatefulSetWithAuditStorage(newCluster()),
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonAuditFileStorageReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &OpenBaoClusterReconciler{
				Client: fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(tt.objects...).
					Build(),
			}

			reconciler.setAuditFileStorageReadyCondition(context.Background(), tt.cluster)

			cond := meta.FindStatusCondition(tt.cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
			if cond == nil {
				t.Fatal("expected AuditFileStorageReady condition")
			}
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("AuditFileStorageReady = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
			}
		})
	}
}

func auditFileStoragePVC(mode corev1.PersistentVolumeAccessMode, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "example-audit", Namespace: "default"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{mode},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: phase},
	}
}

func openBaoStatefulSetWithoutAuditStorage(cluster *openbaov1alpha1.OpenBaoCluster) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Labels:    resourceidentity.Labels(cluster),
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: constants.ContainerBao}},
				},
			},
		},
	}
}

func openBaoStatefulSetWithAuditStorage(cluster *openbaov1alpha1.OpenBaoCluster) *appsv1.StatefulSet {
	sts := openBaoStatefulSetWithoutAuditStorage(cluster)
	sts.Spec.Template.Spec.Volumes = []corev1.Volume{{
		Name: constants.VolumeAuditFileStorage,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: portopenbao.AuditFileStorageClaimName(cluster),
			},
		},
	}}
	sts.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
		Name:        constants.VolumeAuditFileStorage,
		MountPath:   portopenbao.AuditFileStorageMountPath(cluster),
		SubPathExpr: portopenbao.AuditFileStoragePodSubPathExpr,
	}}
	return sts
}
