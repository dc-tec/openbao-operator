package restore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/kubebao/openbao-operator/api/v1alpha1"
)

func TestSubmissionClaimRejectsChangedOrExpiredExecution(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"committed", "expired", "claimed", "replaced", "digest", "size"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			f := newRestoreRecoveryFixture(t)
			require.NoError(t, f.step(t))
			request := f.restore(t)
			uid := request.UID
			switch mutation {
			case "committed":
				request.Status.Execution.Stage = openbaov1alpha1.RestoreExecutionStageCommitted
			case "expired":
				expired := metav1.NewTime(time.Now().Add(-time.Hour))
				request.Status.Execution.PreparedAt = &expired
			case "claimed":
				request.Status.SubmissionClaim = &openbaov1alpha1.RestoreSubmissionClaim{PodUID: "other"}
			case "digest":
				request.Spec.Source.ExpectedDigest = "sha256:expected"
			case "size":
				request.Spec.Source.ExpectedSize = 2
			case "replaced":
				uid = "another-request"
			}
			if mutation == "digest" || mutation == "size" {
				require.NoError(t, f.base.Update(t.Context(), request))
			}
			require.NoError(t, f.base.Status().Update(t.Context(), request))
			err := ClaimRestore(t.Context(), f.base, client.ObjectKeyFromObject(request), uid,
				openbaov1alpha1.RestoreSubmissionClaim{PodUID: "executor", Size: 1})
			require.Error(t, err)
			require.Equal(t, request.Status.SubmissionClaim, f.restore(t).Status.SubmissionClaim)
		})
	}
}
