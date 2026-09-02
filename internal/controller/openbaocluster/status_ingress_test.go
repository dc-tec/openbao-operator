package openbaocluster

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const testIngressStatusClassName = "nginx"

func TestSetIngressIntegrationReadyCondition_FastContract(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

	newIngressClass := func() *networkingv1.IngressClass {
		return &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: testIngressStatusClassName}}
	}
	newIngress := func(addresses ...string) *networkingv1.Ingress {
		published := make([]networkingv1.IngressLoadBalancerIngress, 0, len(addresses))
		for _, address := range addresses {
			published = append(published, networkingv1.IngressLoadBalancerIngress{Hostname: address})
		}
		return &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			Status: networkingv1.IngressStatus{
				LoadBalancer: networkingv1.IngressLoadBalancerStatus{Ingress: published},
			},
		}
	}

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		objects       []client.Object
		wantPresent   bool
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "ingress disabled removes condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Status.Conditions = []metav1.Condition{{
					Type:   string(openbaov1alpha1.ConditionIngressIntegrationReady),
					Status: metav1.ConditionTrue,
					Reason: constants.ReasonIngressIntegrationReady,
				}}
				return cluster
			}(),
			wantPresent: false,
		},
		{
			name: "ingress integration ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				className := testIngressStatusClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:       true,
					ClassName:     &className,
					Host:          "bao.example.test",
					ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
				}
				return cluster
			}(),
			objects:       []client.Object{newIngressClass(), newIngress("lb.example.test")},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonIngressIntegrationReady,
			wantMessageIn: "prerequisites are satisfied",
		},
		{
			name: "ingress class missing is explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				className := testIngressStatusClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:   true,
					ClassName: &className,
					Host:      "bao.example.test",
				}
				return cluster
			}(),
			objects:       []client.Object{newIngress()},
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonIngressClassMissing,
			wantMessageIn: "IngressClass",
		},
		{
			name: "load balancer pending is unknown",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				className := testIngressStatusClassName
				cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
					Enabled:       true,
					ClassName:     &className,
					Host:          "bao.example.test",
					ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
				}
				return cluster
			}(),
			objects:       []client.Object{newIngressClass(), newIngress()},
			wantPresent:   true,
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    constants.ReasonIngressLoadBalancerPending,
			wantMessageIn: "load balancer address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			fakeClient := builder.Build()
			reconciler := &OpenBaoClusterReconciler{
				Client:       fakeClient,
				Applications: newStatusTestApplications(fakeClient, scheme),
			}

			reconciler.setIngressIntegrationReadyCondition(context.Background(), tt.cluster)
			assertClusterCondition(
				t,
				tt.cluster,
				openbaov1alpha1.ConditionIngressIntegrationReady,
				tt.wantPresent,
				tt.wantStatus,
				tt.wantReason,
				tt.wantMessageIn,
			)
		})
	}
}
