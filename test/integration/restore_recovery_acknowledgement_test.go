//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestManagedRestartStatusRequiresController(t *testing.T) {
	installRestoreExecutionPolicy(t)
	namespace := newTestNamespace(t)
	request := newClaimRequest(t, namespace, "restart-status")
	before := request.DeepCopy()
	request.Status.Restart = &api.RestoreRestartStatus{Pods: []api.RestoreRestartPod{{Name: "target-0", UID: "pod", StatefulSetUID: "sts"}}}
	require.Eventually(t, func() bool {
		err := k8sClient.Status().Patch(ctx, request, client.MergeFrom(before), client.DryRunAll)
		return err != nil && strings.Contains(err.Error(), "Only the controller can record managed")
	}, 10*time.Second, 100*time.Millisecond)
	require.NoError(t, newControllerClient(t).Status().Patch(ctx, request, client.MergeFrom(before)))
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(request), request))
	require.Equal(t, "target-0", request.Status.Restart.Pods[0].Name)
}

func TestRecordedRestoreDispositionRejectsConflictingAcknowledgement(t *testing.T) {
	installRestoreExecutionPolicy(t)
	namespace := newTestNamespace(t)
	controller := newControllerClient(t)
	for _, disposition := range []api.RestoreAdministratorDisposition{api.RestoreAdministratorResume, api.RestoreAdministratorAbandon} {
		request := newClaimRequest(t, namespace, "recorded-"+strings.ToLower(string(disposition)))
		before := request.DeepCopy()
		request.Status.Phase = api.RestorePhaseUnknown
		request.Status.AdministratorDisposition = disposition
		require.NoError(t, controller.Status().Patch(ctx, request, client.MergeFrom(before)))
		before = request.DeepCopy()
		request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(request.UID) + "/" + string(disposition)}
		require.NoError(t, k8sClient.Patch(ctx, request, client.MergeFrom(before)))
		before = request.DeepCopy()
		other := api.RestoreAdministratorResume
		if disposition == other {
			other = api.RestoreAdministratorAbandon
		}
		request.Annotations[constants.AnnotationRestoreAcknowledge] = string(request.UID) + "/" + string(other)
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			err := k8sClient.Patch(ctx, request, client.MergeFrom(before), client.DryRunAll)
			require.ErrorContains(c, err, "Recovery has already been released")
		}, 10*time.Second, 100*time.Millisecond)
		delete(request.Annotations, constants.AnnotationRestoreAcknowledge)
		require.NoError(t, k8sClient.Patch(ctx, request, client.MergeFrom(before)))
	}

	request := newClaimRequest(t, namespace, "unfinished-restart")
	before := request.DeepCopy()
	request.Status.Phase = api.RestorePhaseUnknown
	request.Status.Restart = &api.RestoreRestartStatus{Pods: []api.RestoreRestartPod{{Name: "target-0", UID: "pod", StatefulSetUID: "sts"}}}
	require.NoError(t, controller.Status().Patch(ctx, request, client.MergeFrom(before)))
	before = request.DeepCopy()
	request.Annotations = map[string]string{constants.AnnotationRestoreAcknowledge: string(request.UID) + "/Abandon"}
	require.NoError(t, k8sClient.Patch(ctx, request, client.MergeFrom(before)))
}
