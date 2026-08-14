package openbaorestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaorestore "github.com/dc-tec/openbao-operator/internal/app/openbaorestore"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func setAdmissionReady(t *testing.T) {
	t.Helper()
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")
	admission.SetAdmissionDependenciesReady(true)
	t.Cleanup(func() {
		admission.SetAdmissionDependenciesReady(false)
	})
}

func TestOpenBaoRestoreReconciler_Reconcile_NotFound(t *testing.T) {
	setAdmissionReady(t)

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &OpenBaoRestoreReconciler{
		Client:            c,
		Scheme:            scheme,
		RestoreReconciler: appopenbaorestore.NewRestoreReconciler(appopenbaorestore.RestoreDependencies{Client: c, Scheme: scheme}),
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "missing",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestOpenBaoRestoreReconciler_AdmissionDependencyLoss(t *testing.T) {
	t.Run("deletion continues", func(t *testing.T) {
		t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")
		admission.SetAdmissionDependenciesReady(false)
		t.Cleanup(func() { admission.SetAdmissionDependenciesReady(false) })

		scheme := runtime.NewScheme()
		require.NoError(t, batchv1.AddToScheme(scheme))
		require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
		now := metav1.Now()
		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "deleting-restore",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers:        []string{openbaov1alpha1.OpenBaoRestoreFinalizer},
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{Cluster: "missing-cluster"},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(restore).Build()
		tracker := admission.NewTracker(c, admission.DefaultDependencies(), admission.DefaultNamePrefixes(), time.Hour)
		tracker.Set(admission.Status{CheckedAt: time.Now(), OverallReady: false})
		r := &OpenBaoRestoreReconciler{
			Client:            c,
			Scheme:            scheme,
			AdmissionTracker:  tracker,
			RestoreReconciler: appopenbaorestore.NewRestoreReconciler(appopenbaorestore.RestoreDependencies{Client: c, APIReader: c, Scheme: scheme}),
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(restore)})
		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, result)
		current := &openbaov1alpha1.OpenBaoRestore{}
		err = c.Get(context.Background(), client.ObjectKeyFromObject(restore), current)
		if err == nil {
			assert.NotContains(t, current.Finalizers, openbaov1alpha1.OpenBaoRestoreFinalizer)
		} else {
			assert.True(t, apierrors.IsNotFound(err))
		}
	})

	t.Run("normal reconcile remains paused", func(t *testing.T) {
		t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")
		admission.SetAdmissionDependenciesReady(false)
		t.Cleanup(func() { admission.SetAdmissionDependenciesReady(false) })

		scheme := runtime.NewScheme()
		require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{Name: "active-restore", Namespace: "default"},
			Spec:       openbaov1alpha1.OpenBaoRestoreSpec{Cluster: "test-cluster"},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(restore).Build()
		tracker := admission.NewTracker(c, admission.DefaultDependencies(), admission.DefaultNamePrefixes(), time.Hour)
		tracker.Set(admission.Status{CheckedAt: time.Now(), OverallReady: false})
		r := &OpenBaoRestoreReconciler{
			Client:            c,
			Scheme:            scheme,
			AdmissionTracker:  tracker,
			RestoreReconciler: appopenbaorestore.NewRestoreReconciler(appopenbaorestore.RestoreDependencies{Client: c, APIReader: c, Scheme: scheme}),
		}

		result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(restore)})
		require.NoError(t, err)
		assert.Equal(t, constants.RequeueShort, result.RequeueAfter)
		current := &openbaov1alpha1.OpenBaoRestore{}
		require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(restore), current))
		assert.Empty(t, current.Finalizers)
	})
}
