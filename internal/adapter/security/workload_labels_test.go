package security

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const imageVerificationFailurePolicyWarn = "Warn"

func TestManagedWorkloadDigestEnforcementRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			want:    false,
		},
		{
			name: "non-hardened profile",
			cluster: buildClusterWithVerification(
				openbaov1alpha1.ProfileDevelopment,
				enabledVerification(""),
				enabledVerification(""),
			),
			want: false,
		},
		{
			name: "hardened profile",
			cluster: buildClusterWithVerification(
				openbaov1alpha1.ProfileHardened,
				enabledVerification(""),
				enabledVerification(""),
			),
			want: true,
		},
		{
			name: "hardened with image verification warn policy still enforces digest requirement",
			cluster: buildClusterWithVerification(
				openbaov1alpha1.ProfileHardened,
				enabledVerification(imageVerificationFailurePolicyWarn),
				enabledVerification(constants.ImageVerificationFailurePolicyBlock),
			),
			want: true,
		},
		{
			name: "hardened with operator image verification disabled still enforces digest requirement",
			cluster: buildClusterWithVerification(
				openbaov1alpha1.ProfileHardened,
				enabledVerification(constants.ImageVerificationFailurePolicyBlock),
				&openbaov1alpha1.ImageVerificationConfig{Enabled: false},
			),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ManagedWorkloadDigestEnforcementRequired(tt.cluster)
			if got != tt.want {
				t.Fatalf("ManagedWorkloadDigestEnforcementRequired() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAddManagedWorkloadSecurityLabels(t *testing.T) {
	t.Parallel()

	cluster := buildClusterWithVerification(
		openbaov1alpha1.ProfileHardened,
		enabledVerification(constants.ImageVerificationFailurePolicyBlock),
		enabledVerification(constants.ImageVerificationFailurePolicyBlock),
	)

	labels := map[string]string{
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	}
	AddManagedWorkloadSecurityLabels(labels, cluster)

	if got := labels[constants.LabelOpenBaoProfile]; got != string(openbaov1alpha1.ProfileHardened) {
		t.Fatalf("profile label = %q, want %q", got, string(openbaov1alpha1.ProfileHardened))
	}

	if got := labels[constants.LabelOpenBaoDigestEnforcement]; got != constants.LabelValueDigestEnforcementRequired {
		t.Fatalf("digest enforcement label = %q, want %q", got, constants.LabelValueDigestEnforcementRequired)
	}
}

func TestAddManagedWorkloadSecurityLabelsDevelopmentProfile(t *testing.T) {
	t.Parallel()

	cluster := buildClusterWithVerification(
		openbaov1alpha1.ProfileDevelopment,
		nil,
		nil,
	)

	labels := map[string]string{
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	}
	AddManagedWorkloadSecurityLabels(labels, cluster)

	if got := labels[constants.LabelOpenBaoProfile]; got != string(openbaov1alpha1.ProfileDevelopment) {
		t.Fatalf("profile label = %q, want %q", got, string(openbaov1alpha1.ProfileDevelopment))
	}

	if _, found := labels[constants.LabelOpenBaoDigestEnforcement]; found {
		t.Fatalf("did not expect digest enforcement label for Development profile")
	}
}

func buildClusterWithVerification(
	profile openbaov1alpha1.Profile,
	imageVerification *openbaov1alpha1.ImageVerificationConfig,
	operatorImageVerification *openbaov1alpha1.ImageVerificationConfig,
) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:                   profile,
			ImageVerification:         imageVerification,
			OperatorImageVerification: operatorImageVerification,
		},
	}
}

func enabledVerification(policy string) *openbaov1alpha1.ImageVerificationConfig {
	return &openbaov1alpha1.ImageVerificationConfig{
		Enabled:       true,
		FailurePolicy: policy,
	}
}
