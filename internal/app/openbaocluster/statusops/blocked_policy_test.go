package statusops

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestApplyPausedPolicy(t *testing.T) {
	now := metav1.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	cluster := fullyConfiguredBlockedPolicyCluster()

	ApplyPausedPolicy(BlockedPolicyInput{
		Cluster:                       cluster,
		CloudUnsealIdentityApplicable: true,
		Now:                           now,
	})

	assert.Equal(t, openbaov1alpha1.ClusterPhaseInitializing, cluster.Status.Phase)
	assert.Zero(t, cluster.Status.ObservedGeneration)
	assertConditionOrder(t, cluster,
		openbaov1alpha1.ConditionAvailable,
		openbaov1alpha1.ConditionDegraded,
		openbaov1alpha1.ConditionTLSReady,
		openbaov1alpha1.ConditionAPIServerNetworkReady,
		openbaov1alpha1.ConditionACMEIntegrationReady,
		openbaov1alpha1.ConditionAuditFileStorageReady,
		openbaov1alpha1.ConditionGatewayIntegrationReady,
		openbaov1alpha1.ConditionIngressIntegrationReady,
		openbaov1alpha1.ConditionBackupConfigurationReady,
		openbaov1alpha1.ConditionCloudUnsealIdentityReady,
		openbaov1alpha1.ConditionUserAccessBootstrap,
	)

	want := []conditionExpectation{
		{openbaov1alpha1.ConditionAvailable, metav1.ConditionUnknown, reasonPaused, "Reconciliation is paused; availability is not being evaluated"},
		{openbaov1alpha1.ConditionDegraded, metav1.ConditionFalse, reasonPaused, "Cluster is paused; no new degradation has been evaluated"},
		{openbaov1alpha1.ConditionTLSReady, metav1.ConditionUnknown, reasonPaused, "TLS readiness is not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionAPIServerNetworkReady, metav1.ConditionUnknown, reasonPaused, "Kubernetes API egress readiness is not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionACMEIntegrationReady, metav1.ConditionUnknown, reasonPaused, "ACME integration prerequisites are not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionAuditFileStorageReady, metav1.ConditionUnknown, reasonPaused, "Audit file storage readiness is not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionGatewayIntegrationReady, metav1.ConditionUnknown, reasonPaused, "Gateway integration prerequisites are not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionIngressIntegrationReady, metav1.ConditionUnknown, reasonPaused, "Ingress integration prerequisites are not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionBackupConfigurationReady, metav1.ConditionUnknown, reasonPaused, "Backup Job prerequisites are not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionCloudUnsealIdentityReady, metav1.ConditionUnknown, reasonPaused, "Cloud KMS unseal identity prerequisites are not being evaluated while reconciliation is paused"},
		{openbaov1alpha1.ConditionUserAccessBootstrap, metav1.ConditionFalse, ReasonDisabled, "Self-init is disabled; user access bootstrap heuristics are not evaluated"},
	}
	assertConditionExpectations(t, cluster, now, want)
	assert.Nil(t, meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady)))
	assert.Nil(t, meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady)))
}

func TestApplyProfileNotSetPolicy(t *testing.T) {
	now := metav1.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	cluster := fullyConfiguredBlockedPolicyCluster()

	ApplyProfileNotSetPolicy(BlockedPolicyInput{
		Cluster:                       cluster,
		CloudUnsealIdentityApplicable: true,
		Now:                           now,
	})

	assert.Equal(t, openbaov1alpha1.ClusterPhaseInitializing, cluster.Status.Phase)
	assert.Zero(t, cluster.Status.ObservedGeneration)
	assertConditionOrder(t, cluster,
		openbaov1alpha1.ConditionAvailable,
		openbaov1alpha1.ConditionDegraded,
		openbaov1alpha1.ConditionTLSReady,
		openbaov1alpha1.ConditionAPIServerNetworkReady,
		openbaov1alpha1.ConditionACMEIntegrationReady,
		openbaov1alpha1.ConditionAuditFileStorageReady,
		openbaov1alpha1.ConditionGatewayIntegrationReady,
		openbaov1alpha1.ConditionIngressIntegrationReady,
		openbaov1alpha1.ConditionBackupConfigurationReady,
		openbaov1alpha1.ConditionCloudUnsealIdentityReady,
		openbaov1alpha1.ConditionProductionReady,
		openbaov1alpha1.ConditionUserAccessBootstrap,
	)

	want := []conditionExpectation{
		{openbaov1alpha1.ConditionAvailable, metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set"},
		{openbaov1alpha1.ConditionDegraded, metav1.ConditionTrue, ReasonProfileNotSet, "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment"},
		{openbaov1alpha1.ConditionTLSReady, metav1.ConditionUnknown, ReasonProfileNotSet, "TLS readiness is not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionAPIServerNetworkReady, metav1.ConditionUnknown, ReasonProfileNotSet, "Kubernetes API egress readiness is not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionACMEIntegrationReady, metav1.ConditionUnknown, ReasonProfileNotSet, "ACME integration prerequisites are not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionAuditFileStorageReady, metav1.ConditionUnknown, ReasonProfileNotSet, "Audit file storage readiness is not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionGatewayIntegrationReady, metav1.ConditionUnknown, ReasonProfileNotSet, "Gateway integration prerequisites are not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionIngressIntegrationReady, metav1.ConditionUnknown, ReasonProfileNotSet, "Ingress integration prerequisites are not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionBackupConfigurationReady, metav1.ConditionUnknown, ReasonProfileNotSet, "Backup Job prerequisites are not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionCloudUnsealIdentityReady, metav1.ConditionUnknown, ReasonProfileNotSet, "Cloud KMS unseal identity prerequisites are not being evaluated until spec.profile is set"},
		{openbaov1alpha1.ConditionProductionReady, metav1.ConditionFalse, ReasonProfileNotSet, "Cluster cannot be considered production-ready until spec.profile is explicitly set"},
		{openbaov1alpha1.ConditionUserAccessBootstrap, metav1.ConditionFalse, ReasonDisabled, "Self-init is disabled; user access bootstrap heuristics are not evaluated"},
	}
	assertConditionExpectations(t, cluster, now, want)
	assert.Nil(t, meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady)))
}

func TestApplyPausedPolicyPreservesProductionReady(t *testing.T) {
	productionReady := metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 2,
		LastTransitionTime: metav1.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		Reason:             "ProductionReady",
		Message:            "existing production readiness result",
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 7},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Conditions: []metav1.Condition{productionReady},
		},
	}

	ApplyPausedPolicy(BlockedPolicyInput{
		Cluster: cluster,
		Now:     metav1.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC),
	})

	assert.Equal(t, &productionReady, meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady)))
}

func TestBlockedPoliciesRemoveInapplicableConditions(t *testing.T) {
	tests := []struct {
		name  string
		apply func(BlockedPolicyInput)
	}{
		{name: "paused", apply: ApplyPausedPolicy},
		{name: "profile not set", apply: ApplyProfileNotSetPolicy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			untouched := metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionACMECacheReady),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 2,
				LastTransitionTime: metav1.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
				Reason:             "ACMECacheReady",
				Message:            "existing ACME cache result",
			}
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 7},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: append([]metav1.Condition{untouched}, optionalBlockedConditions()...),
				},
			}

			tt.apply(BlockedPolicyInput{
				Cluster: cluster,
				Now:     metav1.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC),
			})

			for _, conditionType := range []openbaov1alpha1.ConditionType{
				openbaov1alpha1.ConditionACMEIntegrationReady,
				openbaov1alpha1.ConditionAuditFileStorageReady,
				openbaov1alpha1.ConditionGatewayIntegrationReady,
				openbaov1alpha1.ConditionIngressIntegrationReady,
				openbaov1alpha1.ConditionBackupConfigurationReady,
				openbaov1alpha1.ConditionCloudUnsealIdentityReady,
			} {
				assert.Nil(t, meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType)), "condition %s", conditionType)
			}
			assert.Equal(t, &untouched, meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady)))
		})
	}
}

func TestBlockedPoliciesPreserveExistingPhaseAndOtherFields(t *testing.T) {
	tests := []struct {
		name  string
		apply func(BlockedPolicyInput)
	}{
		{name: "paused", apply: ApplyPausedPolicy},
		{name: "profile not set", apply: ApplyProfileNotSetPolicy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := fullyConfiguredBlockedPolicyCluster()
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
			cluster.Status.ActiveLeader = "example-0"
			cluster.Status.ReadyReplicas = 3
			cluster.Status.CurrentVersion = "2.4.1"
			cluster.Status.Initialized = true
			before := cluster.DeepCopy()

			tt.apply(BlockedPolicyInput{
				Cluster:                       cluster,
				CloudUnsealIdentityApplicable: true,
				Now:                           metav1.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC),
			})

			assert.Equal(t, openbaov1alpha1.ClusterPhaseRunning, cluster.Status.Phase)
			afterWithoutOwnedFields := cluster.DeepCopy()
			afterWithoutOwnedFields.Status.Conditions = before.Status.Conditions
			assert.Equal(t, before, afterWithoutOwnedFields)
		})
	}
}

func TestBlockedPoliciesPreserveOrAdvanceTransitionTimes(t *testing.T) {
	oldTime := metav1.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	now := metav1.Date(2026, 9, 2, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name            string
		apply           func(BlockedPolicyInput)
		availableStatus metav1.ConditionStatus
	}{
		{name: "paused", apply: ApplyPausedPolicy, availableStatus: metav1.ConditionUnknown},
		{name: "profile not set", apply: ApplyProfileNotSetPolicy, availableStatus: metav1.ConditionFalse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 7},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:               string(openbaov1alpha1.ConditionAvailable),
							Status:             tt.availableStatus,
							ObservedGeneration: 3,
							LastTransitionTime: oldTime,
							Reason:             "OldReason",
							Message:            "old message",
						},
						{
							Type:               string(openbaov1alpha1.ConditionTLSReady),
							Status:             metav1.ConditionFalse,
							ObservedGeneration: 3,
							LastTransitionTime: oldTime,
							Reason:             "TLSSecretMissing",
							Message:            "old message",
						},
						{
							Type:               string(openbaov1alpha1.ConditionUserAccessBootstrap),
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 3,
							LastTransitionTime: oldTime,
							Reason:             ReasonUserAccessConfigured,
							Message:            "old message",
						},
					},
				},
			}

			tt.apply(BlockedPolicyInput{Cluster: cluster, Now: now})

			available := requireCondition(t, cluster, openbaov1alpha1.ConditionAvailable)
			assert.Equal(t, oldTime, available.LastTransitionTime)
			assert.Equal(t, int64(7), available.ObservedGeneration)
			tlsReady := requireCondition(t, cluster, openbaov1alpha1.ConditionTLSReady)
			assert.NotEqual(t, oldTime, tlsReady.LastTransitionTime)
			userAccess := requireCondition(t, cluster, openbaov1alpha1.ConditionUserAccessBootstrap)
			assert.Equal(t, now, userAccess.LastTransitionTime)
		})
	}
}

type conditionExpectation struct {
	conditionType openbaov1alpha1.ConditionType
	status        metav1.ConditionStatus
	reason        string
	message       string
}

func fullyConfiguredBlockedPolicyCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Generation: 7},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeACME,
			},
			AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{},
			Gateway:          &openbaov1alpha1.GatewayConfig{Enabled: true},
			Ingress:          &openbaov1alpha1.IngressConfig{Enabled: true},
			Backup:           &openbaov1alpha1.BackupSchedule{},
		},
	}
}

func optionalBlockedConditions() []metav1.Condition {
	conditionTypes := []openbaov1alpha1.ConditionType{
		openbaov1alpha1.ConditionACMEIntegrationReady,
		openbaov1alpha1.ConditionAuditFileStorageReady,
		openbaov1alpha1.ConditionGatewayIntegrationReady,
		openbaov1alpha1.ConditionIngressIntegrationReady,
		openbaov1alpha1.ConditionBackupConfigurationReady,
		openbaov1alpha1.ConditionCloudUnsealIdentityReady,
	}
	conditions := make([]metav1.Condition, 0, len(conditionTypes))
	for _, conditionType := range conditionTypes {
		conditions = append(conditions, metav1.Condition{
			Type:               string(conditionType),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: 2,
			LastTransitionTime: metav1.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
			Reason:             "OldReason",
			Message:            "old message",
		})
	}
	return conditions
}

func assertConditionOrder(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster, want ...openbaov1alpha1.ConditionType) {
	t.Helper()
	got := make([]string, 0, len(cluster.Status.Conditions))
	for _, condition := range cluster.Status.Conditions {
		got = append(got, condition.Type)
	}
	wantStrings := make([]string, 0, len(want))
	for _, conditionType := range want {
		wantStrings = append(wantStrings, string(conditionType))
	}
	assert.Equal(t, wantStrings, got)
}

func assertConditionExpectations(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	userAccessTime metav1.Time,
	want []conditionExpectation,
) {
	t.Helper()
	for _, expectation := range want {
		condition := requireCondition(t, cluster, expectation.conditionType)
		assert.Equal(t, expectation.status, condition.Status, "condition %s status", expectation.conditionType)
		assert.Equal(t, expectation.reason, condition.Reason, "condition %s reason", expectation.conditionType)
		assert.Equal(t, expectation.message, condition.Message, "condition %s message", expectation.conditionType)
		assert.Equal(t, cluster.Generation, condition.ObservedGeneration, "condition %s observed generation", expectation.conditionType)
		if expectation.conditionType == openbaov1alpha1.ConditionUserAccessBootstrap {
			assert.Equal(t, userAccessTime, condition.LastTransitionTime)
		} else {
			assert.False(t, condition.LastTransitionTime.IsZero(), "condition %s transition time", expectation.conditionType)
		}
	}
}

func requireCondition(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
) *metav1.Condition {
	t.Helper()
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	require.NotNil(t, condition, "condition %s", conditionType)
	return condition
}
