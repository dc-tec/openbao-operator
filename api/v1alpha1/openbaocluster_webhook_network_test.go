package v1alpha1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateNetwork_APIServerCIDRNormalizedWarns(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 1,
			TLS: TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			Network: &NetworkConfig{
				APIServerCIDR: "10.43.0.1/16",
				APIServerEndpointIPs: []string{
					"192.168.166.2",
				},
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v, want nil", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("ValidateCreate() expected warnings, got none")
	}
}

func TestValidateNetwork_APIServerCIDRInvalidErrors(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 1,
			TLS: TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			Network: &NetworkConfig{
				APIServerCIDR: "not-a-cidr",
			},
		},
	}

	_, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Fatalf("ValidateCreate() expected error for invalid CIDR, got nil")
	}
}
