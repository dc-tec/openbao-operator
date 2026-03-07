package openbaocluster

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSetTLSReadyCondition(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		objects       []runtime.Object
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "tls disabled",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.TLS.Enabled = false
				return cluster
			}(),
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonDisabled,
			wantMessageIn: "disabled",
		},
		{
			name: "acme mode",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
				return cluster
			}(),
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    reasonUnknown,
			wantMessageIn: "ACME",
		},
		{
			name:          "missing secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonTLSSecretMissing,
			wantMessageIn: "not present yet",
		},
		{
			name:          "invalid secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			objects:       []runtime.Object{&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example" + constants.SuffixTLSServer, Namespace: "default"}, Data: map[string][]byte{"tls.crt": []byte("cert")}}},
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonTLSSecretInvalid,
			wantMessageIn: "missing required keys",
		},
		{
			name:          "valid secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			objects:       []runtime.Object{&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example" + constants.SuffixTLSServer, Namespace: "default"}, Data: map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")}}},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    reasonReady,
			wantMessageIn: "provisioned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			reconciler := &OpenBaoClusterReconciler{Client: builder.Build()}

			reconciler.setTLSReadyCondition(context.Background(), tt.cluster)
			cond := meta.FindStatusCondition(tt.cluster.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			if cond == nil {
				t.Fatal("expected TLSReady condition")
			}
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("TLSReady condition = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
			}
			if tt.wantMessageIn != "" && !strings.Contains(cond.Message, tt.wantMessageIn) {
				t.Fatalf("message = %q, want substring %q", cond.Message, tt.wantMessageIn)
			}
		})
	}
}
