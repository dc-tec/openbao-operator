package openbaocluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSetAPIServerNetworkReadyCondition(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	tests := []struct {
		name          string
		envHost       string
		cluster       *openbaov1alpha1.OpenBaoCluster
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name:          "service vip only is unknown",
			envHost:       "10.43.0.1",
			cluster:       newOpenBaoClusterStatusTestObject(),
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    ReasonAPIServerEndpointIPsRecommended,
			wantMessageIn: "apiServerEndpointIPs",
		},
		{
			name: "explicit endpoint ips are ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					APIServerCIDR:        "10.43.0.1/32",
					APIServerEndpointIPs: []string{"192.168.166.2"},
				}
				return cluster
			}(),
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonAPIServerNetworkReady,
			wantMessageIn: "192.168.166.2",
		},
		{
			name: "invalid cidr is false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					APIServerCIDR: "not-a-cidr",
				}
				return cluster
			}(),
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonAPIServerNetworkConfigurationInvalid,
			wantMessageIn: "spec.network.apiServerCIDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envHost != "" {
				t.Setenv("KUBERNETES_SERVICE_HOST", tt.envHost)
			} else {
				t.Setenv("KUBERNETES_SERVICE_HOST", "")
			}

			reconciler := &OpenBaoClusterReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				ControllerRuntime: ControllerRuntime{
					Scheme: scheme,
				},
			}

			reconciler.setAPIServerNetworkReadyCondition(context.Background(), tt.cluster)
			assertClusterCondition(
				t,
				tt.cluster,
				openbaov1alpha1.ConditionAPIServerNetworkReady,
				true,
				tt.wantStatus,
				tt.wantReason,
				tt.wantMessageIn,
			)
		})
	}
}
