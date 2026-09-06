package upgrade

import (
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRequestAcknowledgementsApplyTo(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name             string
		acknowledgements RequestAcknowledgements
		want             openbaov1alpha1.UpgradeRequestStatus
	}{
		{
			name: "empty",
			want: openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "retry", LastHandledPromote: "promote", LastHandledRollback: "rollback"},
		},
		{
			name:             "blank",
			acknowledgements: RequestAcknowledgements{Retry: " \t", Promote: "\n", Rollback: " "},
			want:             openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "retry", LastHandledPromote: "promote", LastHandledRollback: "rollback"},
		},
		{
			name:             "retry",
			acknowledgements: RequestAcknowledgements{Retry: " next "},
			want:             openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "next", LastHandledPromote: "promote", LastHandledRollback: "rollback"},
		},
		{
			name:             "promote",
			acknowledgements: RequestAcknowledgements{Promote: " next "},
			want:             openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "retry", LastHandledPromote: "next", LastHandledRollback: "rollback"},
		},
		{
			name:             "rollback",
			acknowledgements: RequestAcknowledgements{Rollback: " next "},
			want:             openbaov1alpha1.UpgradeRequestStatus{LastHandledRetry: "retry", LastHandledPromote: "promote", LastHandledRollback: "next"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status := openbaov1alpha1.OpenBaoClusterStatus{UpgradeRequests: &openbaov1alpha1.UpgradeRequestStatus{
				LastHandledRetry: "retry", LastHandledPromote: "promote", LastHandledRollback: "rollback",
			}}
			tt.acknowledgements.ApplyTo(&status)
			require.Equal(t, &tt.want, status.UpgradeRequests)
		})
	}
}

func TestRequestAcknowledgementsMerge(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		first RequestAcknowledgements
		next  RequestAcknowledgements
		want  RequestAcknowledgements
	}{
		{name: "no requests"},
		{
			name:  "different managers",
			first: RequestAcknowledgements{Rollback: "rollback"},
			next:  RequestAcknowledgements{Retry: " retry ", Promote: "promote"},
			want:  RequestAcknowledgements{Retry: "retry", Promote: "promote", Rollback: "rollback"},
		},
		{
			name:  "blank does not clear",
			first: RequestAcknowledgements{Retry: "retry", Promote: "promote", Rollback: "rollback"},
			next:  RequestAcknowledgements{Retry: " ", Promote: "\t", Rollback: "\n"},
			want:  RequestAcknowledgements{Retry: "retry", Promote: "promote", Rollback: "rollback"},
		},
		{
			name:  "later handled token",
			first: RequestAcknowledgements{Retry: "earlier"},
			next:  RequestAcknowledgements{Retry: "later"},
			want:  RequestAcknowledgements{Retry: "later"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.first
			got.Merge(tt.next)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRequestAcknowledgementsEmptyLeavesNilStatus(t *testing.T) {
	t.Parallel()
	for _, acknowledgements := range []RequestAcknowledgements{{}, {Retry: " ", Promote: "\t", Rollback: "\n"}} {
		status := openbaov1alpha1.OpenBaoClusterStatus{}
		require.True(t, acknowledgements.IsEmpty())
		acknowledgements.ApplyTo(&status)
		require.Nil(t, status.UpgradeRequests)
	}
	for _, acknowledgements := range []RequestAcknowledgements{{Retry: "retry"}, {Promote: "promote"}, {Rollback: "rollback"}} {
		status := openbaov1alpha1.OpenBaoClusterStatus{}
		require.False(t, acknowledgements.IsEmpty())
		acknowledgements.ApplyTo(&status)
		require.NotNil(t, status.UpgradeRequests)
	}
}
