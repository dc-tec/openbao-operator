package bluegreen

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestRollbackRunID_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cluster  *openbaov1alpha1.OpenBaoCluster
		expected string
	}{
		{
			name: "default rollback run id without bluegreen",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{},
			},
			expected: "rollback",
		},
		{
			name: "default rollback run id when attempt is zero",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{RollbackAttempt: 0},
				},
			},
			expected: "rollback",
		},
		{
			name: "retry rollback run id when attempt is positive",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{RollbackAttempt: 3},
				},
			},
			expected: "rollback-retry-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rollbackRunID(tt.cluster); got != tt.expected {
				t.Fatalf("rollbackRunID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestShouldHaltForBreakGlass_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cluster  *openbaov1alpha1.OpenBaoCluster
		expected bool
	}{
		{
			name: "does not halt when breakglass status is absent",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec:   openbaov1alpha1.OpenBaoClusterSpec{},
				Status: openbaov1alpha1.OpenBaoClusterStatus{},
			},
			expected: false,
		},
		{
			name: "does not halt when breakglass is inactive",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: false,
						Nonce:  "nonce",
					},
				},
			},
			expected: false,
		},
		{
			name: "does not halt when breakglass was acknowledged",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
					},
				},
			},
			expected: false,
		},
		{
			name: "halts when breakglass is active and not acknowledged",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "other"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
					},
				},
			},
			expected: true,
		},
		{
			name: "halts when ack is lexicographically smaller but not equal",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "z",
					},
				},
			},
			expected: true,
		},
	}

	mgr := &Manager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := mgr.shouldHaltForBreakGlass(logr.Discard(), tt.cluster); got != tt.expected {
				t.Fatalf("shouldHaltForBreakGlass() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleBreakGlassAck_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		cluster             *openbaov1alpha1.OpenBaoCluster
		wantHandled         bool
		wantRequeueShort    bool
		wantActive          bool
		wantRollbackAttempt int32
		wantFailureCount    int32
		wantLastFailure     string
	}{
		{
			name: "noop when breakglass is absent",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{},
			},
			wantHandled:      false,
			wantRequeueShort: false,
		},
		{
			name: "holds when breakglass nonce does not match ack",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "different"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
					},
				},
			},
			wantHandled:      true,
			wantRequeueShort: false,
			wantActive:       true,
		},
		{
			name: "holds when breakglass nonce is empty",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: ""},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
					},
				},
			},
			wantHandled:      true,
			wantRequeueShort: false,
			wantActive:       true,
		},
		{
			name: "holds when nonce mismatch is lexicographically greater",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "z"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "a",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
					},
				},
			},
			wantHandled:      true,
			wantRequeueShort: false,
			wantActive:       true,
		},
		{
			name: "acknowledges rollback consensus breakglass and bumps rollback attempt",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
					},
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:           openbaov1alpha1.PhaseRollingBack,
						RollbackAttempt: 1,
						JobFailureCount: 2,
						LastJobFailure:  "job failed",
					},
				},
			},
			wantHandled:         true,
			wantRequeueShort:    true,
			wantActive:          false,
			wantRollbackAttempt: 2,
			wantFailureCount:    0,
			wantLastFailure:     "",
		},
		{
			name: "acknowledges rollback cleanup peer-removal breakglass and bumps rollback attempt",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-b"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-b",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackCleanupPeerRemovalFailed,
					},
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:           openbaov1alpha1.PhaseRollbackCleanup,
						RollbackAttempt: 2,
						JobFailureCount: 1,
						LastJobFailure:  "remove green peers failed",
					},
				},
			},
			wantHandled:         true,
			wantRequeueShort:    true,
			wantActive:          false,
			wantRollbackAttempt: 3,
			wantFailureCount:    0,
			wantLastFailure:     "",
		},
		{
			name: "acknowledges rollback reason without bluegreen status",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
					},
					BlueGreen: nil,
				},
			},
			wantHandled:      true,
			wantRequeueShort: true,
			wantActive:       false,
		},
		{
			name: "does not bump rollback attempt when phase is not rolling back",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
						Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
					},
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:           openbaov1alpha1.PhaseDeployingGreen,
						RollbackAttempt: 5,
						JobFailureCount: 3,
						LastJobFailure:  "still-failed",
					},
				},
			},
			wantHandled:         true,
			wantRequeueShort:    true,
			wantActive:          false,
			wantRollbackAttempt: 5,
			wantFailureCount:    3,
			wantLastFailure:     "still-failed",
		},
		{
			name: "acknowledges non-rollback breakglass without rollback counter update",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
						Reason: openbaov1alpha1.BreakGlassReason("ManualIntervention"),
					},
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:           openbaov1alpha1.PhaseSyncing,
						RollbackAttempt: 3,
					},
				},
			},
			wantHandled:         true,
			wantRequeueShort:    true,
			wantActive:          false,
			wantRollbackAttempt: 3,
		},
		{
			name: "does not bump rollback attempt when reason is lexicographically greater but different",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{BreakGlassAck: "nonce-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active: true,
						Nonce:  "nonce-a",
						Reason: openbaov1alpha1.BreakGlassReason("zzzz"),
					},
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:           openbaov1alpha1.PhaseRollingBack,
						RollbackAttempt: 4,
						JobFailureCount: 1,
						LastJobFailure:  "previous",
					},
				},
			},
			wantHandled:         true,
			wantRequeueShort:    true,
			wantActive:          false,
			wantRollbackAttempt: 4,
			wantFailureCount:    1,
			wantLastFailure:     "previous",
		},
	}

	mgr := &Manager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handled, result := mgr.handleBreakGlassAck(logr.Discard(), tt.cluster)
			if handled != tt.wantHandled {
				t.Fatalf("handled = %v, want %v", handled, tt.wantHandled)
			}

			if tt.wantRequeueShort {
				if result.RequeueAfter != constants.RequeueShort {
					t.Fatalf("RequeueAfter = %s, want %s", result.RequeueAfter, constants.RequeueShort)
				}
			} else if result.RequeueAfter != 0 {
				t.Fatalf("RequeueAfter = %s, want 0", result.RequeueAfter)
			}

			if tt.cluster.Status.BreakGlass != nil {
				if tt.cluster.Status.BreakGlass.Active != tt.wantActive {
					t.Fatalf("BreakGlass.Active = %v, want %v", tt.cluster.Status.BreakGlass.Active, tt.wantActive)
				}
				if tt.wantHandled && tt.wantActive == false && tt.cluster.Status.BreakGlass.AcknowledgedAt == nil {
					t.Fatalf("AcknowledgedAt should be set after successful ack")
				}
			}

			if tt.cluster.Status.BlueGreen != nil {
				if tt.cluster.Status.BlueGreen.RollbackAttempt != tt.wantRollbackAttempt {
					t.Fatalf("RollbackAttempt = %d, want %d", tt.cluster.Status.BlueGreen.RollbackAttempt, tt.wantRollbackAttempt)
				}
				if tt.cluster.Status.BlueGreen.JobFailureCount != tt.wantFailureCount {
					t.Fatalf("JobFailureCount = %d, want %d", tt.cluster.Status.BlueGreen.JobFailureCount, tt.wantFailureCount)
				}
				if tt.cluster.Status.BlueGreen.LastJobFailure != tt.wantLastFailure {
					t.Fatalf("LastJobFailure = %q, want %q", tt.cluster.Status.BlueGreen.LastJobFailure, tt.wantLastFailure)
				}
			}
		})
	}
}

func TestEnterBreakGlassRollbackConsensusRepairFailed_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		cluster                *openbaov1alpha1.OpenBaoCluster
		jobName                string
		wantReason             openbaov1alpha1.BreakGlassReason
		wantMessageContains    string
		wantStepContains       string
		wantNonceInitialized   bool
		wantKeepsExistingNonce bool
	}{
		{
			name: "sets breakglass details when inactive",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "tenant-a"},
			},
			jobName:              "repair-job-a",
			wantReason:           openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
			wantMessageContains:  "repair-job-a failed",
			wantStepContains:     "patch openbaocluster cluster-a",
			wantNonceInitialized: true,
		},
		{
			name: "does not overwrite active breakglass",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "tenant-a"},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					BreakGlass: &openbaov1alpha1.BreakGlassStatus{
						Active:  true,
						Nonce:   "existing-nonce",
						Message: "existing message",
					},
				},
			},
			jobName:                "repair-job-b",
			wantKeepsExistingNonce: true,
		},
	}

	mgr := &Manager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr.enterBreakGlassRollbackConsensusRepairFailed(logr.Discard(), tt.cluster, tt.jobName)

			if tt.cluster.Status.BreakGlass == nil {
				t.Fatalf("BreakGlass status should be initialized")
			}

			if tt.wantKeepsExistingNonce {
				if tt.cluster.Status.BreakGlass.Nonce != "existing-nonce" {
					t.Fatalf("Nonce = %q, want %q", tt.cluster.Status.BreakGlass.Nonce, "existing-nonce")
				}
				return
			}

			if tt.cluster.Status.BreakGlass.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", tt.cluster.Status.BreakGlass.Reason, tt.wantReason)
			}
			if !strings.Contains(tt.cluster.Status.BreakGlass.Message, tt.wantMessageContains) {
				t.Fatalf("Message = %q, want substring %q", tt.cluster.Status.BreakGlass.Message, tt.wantMessageContains)
			}
			stepsJoined := strings.Join(tt.cluster.Status.BreakGlass.Steps, "\n")
			if !strings.Contains(stepsJoined, tt.wantStepContains) {
				t.Fatalf("Steps missing expected substring %q", tt.wantStepContains)
			}
			if tt.wantNonceInitialized {
				if tt.cluster.Status.BreakGlass.Nonce == "" {
					t.Fatalf("Nonce should be initialized")
				}
				if tt.cluster.Status.BreakGlass.EnteredAt == nil {
					t.Fatalf("EnteredAt should be set")
				}
			}
		})
	}
}

func TestEnterBreakGlassRollbackCleanupPeerRemovalFailed_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		cluster             *openbaov1alpha1.OpenBaoCluster
		jobName             string
		wantReason          openbaov1alpha1.BreakGlassReason
		wantMessageContains string
		wantStepContains    string
	}{
		{
			name: "sets breakglass details for rollback cleanup peer removal",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-b", Namespace: "tenant-b"},
			},
			jobName:             "remove-green-peers-job",
			wantReason:          openbaov1alpha1.BreakGlassReasonRollbackCleanupPeerRemovalFailed,
			wantMessageContains: "remove-green-peers-job failed",
			wantStepContains:    "Remove any stale Green peers manually",
		},
	}

	mgr := &Manager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mgr.enterBreakGlassRollbackCleanupPeerRemovalFailed(logr.Discard(), tt.cluster, tt.jobName)

			if tt.cluster.Status.BreakGlass == nil {
				t.Fatalf("BreakGlass status should be initialized")
			}
			if tt.cluster.Status.BreakGlass.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", tt.cluster.Status.BreakGlass.Reason, tt.wantReason)
			}
			if !strings.Contains(tt.cluster.Status.BreakGlass.Message, tt.wantMessageContains) {
				t.Fatalf("Message = %q, want substring %q", tt.cluster.Status.BreakGlass.Message, tt.wantMessageContains)
			}
			stepsJoined := strings.Join(tt.cluster.Status.BreakGlass.Steps, "\n")
			if !strings.Contains(stepsJoined, tt.wantStepContains) {
				t.Fatalf("Steps missing expected substring %q", tt.wantStepContains)
			}
			if tt.cluster.Status.BreakGlass.Nonce == "" {
				t.Fatalf("Nonce should be initialized")
			}
			if tt.cluster.Status.BreakGlass.EnteredAt == nil {
				t.Fatalf("EnteredAt should be set")
			}
		})
	}
}

func TestNewBreakGlassNonce_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "nonce has expected hex length"},
		{name: "nonce is valid hex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nonce := newBreakGlassNonce()
			if len(nonce) != breakGlassNonceBytes*2 {
				t.Fatalf("len(nonce) = %d, want %d", len(nonce), breakGlassNonceBytes*2)
			}
			if _, err := hex.DecodeString(nonce); err != nil {
				t.Fatalf("nonce should be hex encoded, got err: %v", err)
			}
		})
	}
}
