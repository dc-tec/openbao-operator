package main

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openbao/operator/internal/constants"
)

// testClusterName is the cluster name used in drift predicate tests.
const testClusterName = "test-cluster"

func TestDriftPredicate_UpdateFunc(t *testing.T) {
	clusterName := testClusterName
	now := time.Now()
	recentTime := now.Add(-2 * time.Second) // Within grace period (5s)
	oldTime := now.Add(-7 * time.Second)    // Outside grace period but recent enough for annotation check (10s)
	veryOldTime := now.Add(-1 * time.Hour)  // Very old

	// Create predicate function
	pred := buildDriftPredicate(clusterName)

	tests := []struct {
		name     string
		event    event.UpdateEvent
		wantPass bool
	}{
		{
			name: "non-operator update should pass",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
					{Manager: "kubectl", Time: now},
				}),
			},
			wantPass: true,
		},
		{
			name: "operator update should be filtered",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: now},
				}),
			},
			wantPass: false,
		},
		{
			name: "operator update within grace period should be filtered",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: recentTime},
				}),
			},
			wantPass: false,
		},
		{
			name: "maintenance annotation added should pass",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"openbao.org/maintenance": "true",
				}, []managedField{
					// Old enough to pass grace period, recent enough for annotation check
					{Manager: "openbao-operator", Time: oldTime},
				}),
			},
			wantPass: true,
		},
		{
			name: "maintenance annotation changed should pass",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{
					"openbao.org/maintenance": "false",
				}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"openbao.org/maintenance": "true",
				}, []managedField{
					// Old enough to pass grace period, recent enough for annotation check
					{Manager: "openbao-operator", Time: oldTime},
				}),
			},
			wantPass: true,
		},
		{
			name: "maintenance annotation unchanged should be filtered",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{
					"openbao.org/maintenance": "true",
				}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"openbao.org/maintenance": "true",
				}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
			},
			wantPass: false,
		},
		{
			name: "non-operator annotation added should pass",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"custom-annotation": "value",
				}, []managedField{
					// Old enough to pass grace period, recent enough for annotation check
					{Manager: "openbao-operator", Time: oldTime},
				}),
			},
			wantPass: true,
		},
		{
			name: "e2e test annotation should be ignored",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"e2e.openbao.org/test": "value",
				}, []managedField{
					{Manager: "openbao-operator", Time: now},
				}),
			},
			wantPass: false,
		},
		{
			name: "kubectl annotation should be ignored",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"kubectl.kubernetes.io/last-applied-configuration": "{}",
				}, []managedField{
					{Manager: "openbao-operator", Time: now},
				}),
			},
			wantPass: false,
		},
		{
			name: "recent non-operator update in managedFields should pass",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{}, []managedField{
					// Recent non-operator update (< 15s)
					{Manager: "kubectl", Time: now.Add(-10 * time.Second)},
					// Most recent is operator, but old enough to pass grace period
					{Manager: "openbao-operator", Time: oldTime},
				}),
			},
			wantPass: true,
		},
		{
			name: "old non-operator update in managedFields should be filtered",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "kubectl", Time: veryOldTime}, // Old non-operator update
					{Manager: "openbao-operator", Time: now},
				}),
			},
			wantPass: false,
		},
		{
			name: "resource not managed by operator should be filtered",
			event: event.UpdateEvent{
				ObjectOld: newService("other-cluster", map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService("other-cluster", map[string]string{
					"custom-annotation": "value",
				}, []managedField{
					{Manager: "kubectl", Time: now},
				}),
			},
			wantPass: false,
		},
		{
			name: "maintenance-allowed annotation should be ignored",
			event: event.UpdateEvent{
				ObjectOld: newService(clusterName, map[string]string{}, []managedField{
					{Manager: "openbao-operator", Time: oldTime},
				}),
				ObjectNew: newService(clusterName, map[string]string{
					"openbao.org/maintenance-allowed": "true",
				}, []managedField{
					{Manager: "openbao-operator", Time: oldTime}, // Old enough to pass grace period
				}),
			},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pred.UpdateFunc(tt.event)
			if got != tt.wantPass {
				t.Errorf("UpdateFunc() = %v, want %v", got, tt.wantPass)
			}
		})
	}
}

func TestDriftPredicate_CreateFunc(t *testing.T) {
	clusterName := testClusterName
	pred := buildDriftPredicate(clusterName)

	tests := []struct {
		name     string
		event    event.CreateEvent
		wantPass bool
	}{
		{
			name: "managed resource should pass",
			event: event.CreateEvent{
				Object: newService(clusterName, map[string]string{}, nil),
			},
			wantPass: true,
		},
		{
			name: "non-managed resource should be filtered",
			event: event.CreateEvent{
				Object: newService("other-cluster", map[string]string{}, nil),
			},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pred.CreateFunc(tt.event)
			if got != tt.wantPass {
				t.Errorf("CreateFunc() = %v, want %v", got, tt.wantPass)
			}
		})
	}
}

func TestDriftPredicate_DeleteFunc(t *testing.T) {
	clusterName := testClusterName
	pred := buildDriftPredicate(clusterName)

	deleteEvt := event.DeleteEvent{
		Object: newService(clusterName, map[string]string{}, nil),
	}

	got := pred.DeleteFunc(deleteEvt)
	if got != false {
		t.Errorf("DeleteFunc() = %v, want false", got)
	}
}

// Helper functions

type managedField struct {
	Manager string
	Time    time.Time
}

func newService(clusterName string, annotations map[string]string, managedFields []managedField) *corev1.Service {
	labels := map[string]string{
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelAppInstance:  clusterName,
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        clusterName,
			Namespace:   "default",
			Labels:      labels,
			Annotations: annotations,
		},
	}

	// Set managedFields
	if len(managedFields) > 0 {
		fields := make([]metav1.ManagedFieldsEntry, len(managedFields))
		for i, mf := range managedFields {
			fields[i] = metav1.ManagedFieldsEntry{
				Manager:    mf.Manager,
				Operation:  metav1.ManagedFieldsOperationUpdate,
				APIVersion: "v1",
				Time:       &metav1.Time{Time: mf.Time},
			}
		}
		svc.ManagedFields = fields
	}

	return svc
}

// The buildDriftPredicate function is defined in main.go and reused by tests.
// This comment keeps the test file at this location after removal of the duplicate.
