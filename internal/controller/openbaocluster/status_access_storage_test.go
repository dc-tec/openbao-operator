package openbaocluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func stringPtr(s string) *string {
	return &s
}

func TestBuildUserAccessBootstrapCondition(t *testing.T) {
	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantInMessage string
	}{
		{
			name:       "self init disabled",
			cluster:    &openbaov1alpha1.OpenBaoCluster{},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonDisabled,
		},
		{
			name: "self init enabled but only operator oidc",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC:    &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
						Requests: []openbaov1alpha1.SelfInitRequest{
							{
								Name:      "operator-role",
								Operation: openbaov1alpha1.SelfInitOperationCreate,
								Path:      "auth/jwt-operator/role/openbao-operator",
							},
						},
					},
				},
			},
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    ReasonUserAccessUnverified,
			wantInMessage: "spec.selfInit.oidc only bootstraps operator authentication",
		},
		{
			name: "structured auth method recognized",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						Requests: []openbaov1alpha1.SelfInitRequest{
							{
								Name:      "enable-userpass",
								Operation: openbaov1alpha1.SelfInitOperationCreate,
								Path:      "sys/auth/userpass",
								AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
									Type: "userpass",
								},
							},
						},
					},
				},
			},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonUserAccessConfigured,
			wantInMessage: "auth/userpass",
		},
		{
			name: "auth request path recognized",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						Requests: []openbaov1alpha1.SelfInitRequest{
							{
								Name:      "configure-admin-role",
								Operation: openbaov1alpha1.SelfInitOperationCreate,
								Path:      "auth/jwt/role/admin",
							},
						},
					},
				},
			},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonUserAccessConfigured,
			wantInMessage: "auth/jwt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildUserAccessBootstrapCondition(tt.cluster)

			assert.Equal(t, string(openbaov1alpha1.ConditionUserAccessBootstrap), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
			if tt.wantInMessage != "" {
				assert.Contains(t, cond.Message, tt.wantInMessage)
			}
		})
	}
}

func TestBuildStorageConfiguredCondition(t *testing.T) {
	const className = "fast-ssd"

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
		wantInMsg  string
	}{
		{
			name: "explicit storage class before pvc creation",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Storage: openbaov1alpha1.StorageConfig{
						StorageClassName: stringPtr(className),
					},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassConfigured,
			wantInMsg:  "Configured to request",
		},
		{
			name: "default storage class pending",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionUnknown,
			wantReason: ReasonStorageClassPending,
			wantInMsg:  "rely on the default StorageClass",
		},
		{
			name: "default storage class resolved from pvcs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state: &clusterState{
				DataPVCCount:             3,
				DataPVCStorageClassNames: []string{"gp3"},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassDefaulted,
			wantInMsg:  "Using default StorageClass",
		},
		{
			name: "configured storage class mismatch",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Storage: openbaov1alpha1.StorageConfig{
						StorageClassName: stringPtr(className),
					},
				},
			},
			state: &clusterState{
				DataPVCCount:             1,
				DataPVCStorageClassNames: []string{"gp3"},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonStorageClassMismatch,
			wantInMsg:  "does not match",
		},
		{
			name: "inconsistent storage classes across pvcs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state: &clusterState{
				DataPVCCount:             3,
				DataPVCStorageClassNames: []string{"fast", "slow"},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonStorageClassInconsistent,
			wantInMsg:  "inconsistent StorageClass values",
		},
		{
			name: "pvcs created without storage class",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state: &clusterState{
				DataPVCCount:             2,
				DataPVCStorageClassUnset: true,
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassUnset,
			wantInMsg:  "without a StorageClass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildStorageConfiguredCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionStorageConfigured), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
			assert.Contains(t, cond.Message, tt.wantInMsg)
		})
	}
}

func TestBuildReadReplicaStorageConfiguredCondition(t *testing.T) {
	const className = "fast-ssd"

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		state      *clusterState
		wantStatus metav1.ConditionStatus
		wantReason string
		wantInMsg  string
	}{
		{
			name: "no read replicas configured",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonNoReadReplicasConfigured,
			wantInMsg:  "No steady-state read replicas are configured",
		},
		{
			name: "explicit read storage class before pvc creation",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
						Replicas: 2,
						Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
							StorageClassName: stringPtr(className),
						},
					},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonStorageClassConfigured,
			wantInMsg:  "Configured to request",
		},
		{
			name: "default read storage class pending",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
						Replicas: 2,
					},
				},
			},
			state:      &clusterState{},
			wantStatus: metav1.ConditionUnknown,
			wantReason: ReasonStorageClassPending,
			wantInMsg:  "rely on the default StorageClass",
		},
		{
			name: "read storage class mismatch",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{
						Replicas: 2,
						Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
							StorageClassName: stringPtr(className),
						},
					},
				},
			},
			state: &clusterState{
				ReadReplicaDataPVCCount:             2,
				ReadReplicaDataPVCStorageClassNames: []string{"gp3"},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonStorageClassMismatch,
			wantInMsg:  "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := buildReadReplicaStorageConfiguredCondition(tt.cluster, tt.state)

			assert.Equal(t, string(openbaov1alpha1.ConditionReadReplicaStorageConfigured), cond.Type)
			assert.Equal(t, tt.wantStatus, cond.Status)
			assert.Equal(t, tt.wantReason, cond.Reason)
			assert.Contains(t, cond.Message, tt.wantInMsg)
		})
	}
}
