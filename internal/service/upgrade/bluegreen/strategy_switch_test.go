package bluegreen

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func TestBlueRetryJoinLabelSelector_UnrevisionedBlueUsesControllerRevision(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "bao", Namespace: "tenant"},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueControllerRevision: "bao-6d89f76c4b",
			},
		},
	}

	selector := blueRetryJoinLabelSelector(cluster, "")
	for _, expected := range []string{
		appsv1.ControllerRevisionHashLabelKey + "=bao-6d89f76c4b",
		constants.LabelOpenBaoCluster + "=bao",
		constants.LabelOpenBaoWorkloadPool + "=" + constants.LabelValueOpenBaoWorkloadPoolVoter,
	} {
		if !strings.Contains(selector, expected) {
			t.Fatalf("selector %q does not contain %q", selector, expected)
		}
	}

	if got := blueRetryJoinLabelSelector(cluster, "blue123"); got != "" {
		t.Fatalf("revisioned Blue selector override = %q, want empty", got)
	}

	options := greenRenderOptions(cluster, "")
	if !options.RetryJoinAsNonVoter {
		t.Fatal("unrevisioned Green render options RetryJoinAsNonVoter=false, want true")
	}
	if options.RetryJoinLabelSelector != selector {
		t.Fatalf("Green render selector = %q, want %q", options.RetryJoinLabelSelector, selector)
	}
}

func TestGetPodsByRevision_UnrevisionedBlueUsesControllerRevision(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "bao", Namespace: "tenant"},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueControllerRevision: "bao-6d89f76c4b",
			},
		},
	}
	blueLabels := resourceidentity.VoterPodSelectorLabels(cluster)
	blueLabels[appsv1.ControllerRevisionHashLabelKey] = "bao-6d89f76c4b"
	greenLabels := resourceidentity.VoterPodSelectorLabels(cluster)
	greenLabels[appsv1.ControllerRevisionHashLabelKey] = "bao-green-75f84d9b"
	greenLabels[constants.LabelOpenBaoRevision] = "green123"

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	manager := &Manager{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bao-0", Namespace: cluster.Namespace, Labels: blueLabels}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bao-green123-0", Namespace: cluster.Namespace, Labels: greenLabels}},
	).Build()}

	pods, err := manager.getPodsByRevision(context.Background(), cluster, "")
	if err != nil {
		t.Fatalf("getPodsByRevision() error = %v", err)
	}
	if len(pods) != 1 || pods[0].Name != "bao-0" {
		t.Fatalf("unrevisioned Blue pods = %#v, want only bao-0", pods)
	}
}
