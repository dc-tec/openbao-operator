package bluegreen

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func TestCleanupPreservesTargetAfterSpecDrift(t *testing.T) {
	for _, change := range []string{"image", "replicas"} {
		t.Run(change, func(t *testing.T) {
			cluster := newPhaseMachineCluster()
			cluster.Spec.Version = "2.5.5"
			cluster.Spec.Image = "openbao/openbao:2.5.5"
			originalImage := cluster.Spec.Image
			cluster.Spec.Replicas = 3
			cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 1}
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseCleanup
			manager := &Manager{}
			cluster.Status.BlueGreen.GreenRevision = manager.calculateRevision(cluster)
			core.CaptureBlueGreenTarget(cluster)
			objects := []client.Object{cluster, succeededExecutorJob(cluster, ActionRemoveBluePeers)}
			for i := 0; i < 3; i++ {
				pod := newRevisionPod(cluster, cluster.Status.BlueGreen.GreenRevision, fmt.Sprintf("green-%d", i))
				markPodReadyUnsealed(pod)
				if i == 0 {
					pod.Labels[portopenbao.LabelActive] = "true"
				}
				objects = append(objects, pod)
			}
			if change == "image" {
				cluster.Spec.Version = "2.6.2"
				cluster.Spec.Image = "openbao/openbao:2.6.2"
			} else {
				cluster.Spec.Replicas = 5
			}
			scheme := newBlueGreenTestScheme(t)
			manager.client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			manager.reader = manager.client
			manager.scheme = scheme
			handled, _, err := manager.maybeHandleTargetRevisionDrift(t.Context(), logr.Discard(), cluster)
			if err != nil || handled {
				t.Fatalf("drift did not continue cleanup: handled=%v err=%v", handled, err)
			}
			outcome, err := manager.handlePhaseCleanup(t.Context(), logr.Discard(), cluster)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.applyOutcome(t.Context(), logr.Discard(), cluster, outcome); err != nil {
				t.Fatal(err)
			}
			if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseRestoringReadReplicas {
				t.Fatalf("cleanup did not finish original target: phase=%s", cluster.Status.BlueGreen.Phase)
			}
			if cluster.Spec.Replicas != 5 && change == "replicas" {
				t.Fatal("desired replica count was overwritten")
			}
			if cluster.Status.BlueGreen.BlueImage != originalImage {
				t.Fatalf("promoted image=%s, actual Green image=%s", cluster.Status.BlueGreen.BlueImage, originalImage)
			}
		})
	}
}
