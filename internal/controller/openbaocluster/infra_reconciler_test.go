package openbaocluster

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/openbao"
)

func TestHandleScaleDownSafety(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	clusterName := "test-cluster"
	namespace := "default"

	tests := []struct {
		name            string
		currentReplicas int32
		desiredReplicas int32
		victimLeader    bool
		victimError     bool // simulate network error
		expectedError   string
	}{
		{
			name:            "No scale down",
			currentReplicas: 3,
			desiredReplicas: 3,
			victimLeader:    false, // Irrelevant
			expectedError:   "",
		},
		{
			name:            "Scale up",
			currentReplicas: 3,
			desiredReplicas: 4,
			victimLeader:    false, // Irrelevant
			expectedError:   "",
		},
		{
			name:            "Scale down, victim is follower",
			currentReplicas: 3,
			desiredReplicas: 2,
			victimLeader:    false,
			expectedError:   "",
		},
		{
			name:            "Scale down, victim is leader",
			currentReplicas: 3,
			desiredReplicas: 2,
			victimLeader:    true,
			expectedError:   "waiting for leader step-down on test-cluster-2 to complete",
		},
		{
			name:            "Scale down, victim unreachable",
			currentReplicas: 3,
			desiredReplicas: 2,
			victimError:     true,
			expectedError:   "", // Should proceed (fail open)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: namespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: tt.desiredReplicas,
				},
			}

			// Mock StatefulSet
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: namespace,
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: &tt.currentReplicas,
				},
			}

			// Mock Client
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cluster, sts).
				Build()

			// Mock OpenBao Client for victim pod
			clientFunc := func(c *openbaov1alpha1.OpenBaoCluster, podName string) (*openbao.Client, error) {
				if tt.victimError {
					return nil, fmt.Errorf("network error")
				}

				// Mock server to handle health/step-down
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/sys/health":
						if tt.victimLeader {
							// Active Leader: 200 OK, initialized=true, sealed=false, standby=false
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": false}`))
						} else {
							// Follower: 429, initialized=true, sealed=false, standby=true
							w.WriteHeader(http.StatusTooManyRequests)
							_, _ = w.Write([]byte(`{"initialized": true, "sealed": false, "standby": true}`))
						}
					case "/v1/sys/step-down":
						if r.Method == http.MethodPut {
							w.WriteHeader(http.StatusNoContent)
						} else {
							w.WriteHeader(http.StatusMethodNotAllowed)
						}
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				// Note: server is not closed, leaking resources in test but acceptable for short unit test

				// Important: Use server URL
				clientConfig := openbao.ClientConfig{
					BaseURL: server.URL,
					Token:   "root", // required for step-down
				}

				return openbao.NewClient(clientConfig)
			}

			r := &infraReconciler{
				client:           k8sClient,
				scheme:           scheme,
				recorder:         nil, // not needed for this test part
				clientForPodFunc: clientFunc,
			}

			err := r.handleScaleDownSafety(context.Background(), cluster, tt.desiredReplicas, sts)
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}
