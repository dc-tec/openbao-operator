package bluegreen

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/infra"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/security"
)

func TestManager_Reconcile_SkipsWhenNotBlueGreen(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:        "2.4.4",
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{Type: openbaov1alpha1.UpdateStrategyRollingUpdate},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:    true,
			CurrentVersion: "2.4.3",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		Build()
	infraMgr := infra.NewManager(c, scheme, "openbao-operator-system", "", nil, "")
	mgr := NewManager(c, scheme, infraMgr, openbao.ClientConfig{}, security.NewImageVerifier(logr.Discard(), c, nil), security.NewImageVerifier(logr.Discard(), c, nil), "")

	result, err := mgr.Reconcile(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.RequeueAfter > 0 {
		t.Fatalf("expected no requeue")
	}
}
