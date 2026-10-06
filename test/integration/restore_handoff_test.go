//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
	"github.com/dc-tec/openbao-operator/internal/service/workload"
)

func TestRetainedRestoreHandoffAdmissionAndReconcile(t *testing.T) {
	for name, object := range map[string]client.Object{
		"openbao-lock-controller-statefulset-mutations.yaml":         &admissionv1.ValidatingAdmissionPolicy{},
		"openbao-lock-controller-statefulset-mutations-binding.yaml": &admissionv1.ValidatingAdmissionPolicyBinding{},
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "config", "policy", name))
		require.NoError(t, err)
		require.NoError(t, yaml.Unmarshal(data, object))
		require.NoError(t, k8sClient.Create(ctx, object))
		t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), object) })
	}
	namespace := newTestNamespace(t)
	cluster := createMinimalCluster(t, namespace, "handoff")
	cluster.Spec.Replicas = 1
	cluster.Status.Initialized = true
	cluster.Annotations = map[string]string{constants.AnnotationRestoreOrigin: "request", constants.AnnotationRestoreHold: "request"}
	require.NoError(t, k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: resourceidentity.TLSServerSecretName(cluster)}}))
	controller := newControllerClient(t)
	manager := workload.NewManager(controller, k8sScheme, constants.PlatformKubernetes)
	spec := workload.StatefulSetSpec{Name: cluster.Name, Replicas: 1}
	restricted, err := configuration.Render(cluster, configuration.RenderOptions{})
	require.NoError(t, err)
	require.NoError(t, manager.EnsureStatefulSet(ctx, logr.Discard(), cluster, restricted, spec))
	original := &appsv1.StatefulSet{}
	require.NoError(t, controller.Get(ctx, client.ObjectKeyFromObject(cluster), original))
	require.Eventually(t, func() bool {
		changed := original.DeepCopy()
		changed.Spec.Template.Spec.Volumes[0].ConfigMap.Name = "forbidden"
		return controller.Patch(ctx, changed, client.MergeFrom(original), client.DryRunAll) != nil
	}, 10*time.Second, 100*time.Millisecond)
	managed := cluster.DeepCopy()
	delete(managed.Annotations, constants.AnnotationRestoreOrigin)
	normal, err := configuration.Render(managed, configuration.RenderOptions{})
	require.NoError(t, err)
	require.Contains(t, normal, `service_registration "kubernetes"`)
	require.NotContains(t, restricted, `service_registration "kubernetes"`)
	_, err = manager.PrepareRestoreHandoff(ctx, cluster, "replacement-sts", normal)
	require.ErrorContains(t, err, "recorded target StatefulSet")
	hash, err := manager.PrepareRestoreHandoff(ctx, cluster, original.UID, normal)
	require.NoError(t, err)
	prepared := &appsv1.StatefulSet{}
	require.NoError(t, controller.Get(ctx, client.ObjectKeyFromObject(cluster), prepared))
	require.Equal(t, appsv1.OnDeleteStatefulSetStrategyType, prepared.Spec.UpdateStrategy.Type)
	require.Equal(t, hash, prepared.Spec.Template.Annotations[constants.AnnotationConfigHash])
	require.Equal(t, original.Spec.Template.Spec.InitContainers, prepared.Spec.Template.Spec.InitContainers)
	require.Equal(t, original.Spec.Template.Spec.ServiceAccountName, prepared.Spec.Template.Spec.ServiceAccountName)
	require.False(t, *prepared.Spec.Template.Spec.AutomountServiceAccountToken)
	require.Len(t, prepared.Spec.Template.Spec.Volumes, len(original.Spec.Template.Spec.Volumes)+1)
	projection := prepared.Spec.Template.Spec.Volumes[len(prepared.Spec.Template.Spec.Volumes)-1].Projected
	require.Equal(t, "token", projection.Sources[0].ServiceAccountToken.Path)
	require.Equal(t, "kube-root-ca.crt", projection.Sources[1].ConfigMap.Name)

	for name, mutate := range map[string]func(*appsv1.StatefulSet){
		"other audience": func(s *appsv1.StatefulSet) {
			s.Spec.Template.Spec.Volumes[len(s.Spec.Template.Spec.Volumes)-1].Projected.Sources[0].ServiceAccountToken.Audience = "unrelated-audience"
		},
		"other configmap": func(s *appsv1.StatefulSet) {
			s.Spec.Template.Spec.Volumes[len(s.Spec.Template.Spec.Volumes)-1].Projected.Sources[1].ConfigMap.Name = "secret-config"
		},
		"existing volume": func(s *appsv1.StatefulSet) { s.Spec.Template.Spec.Volumes[0].ConfigMap.Name = "unrelated-config" },
		"other mount": func(s *appsv1.StatefulSet) {
			m := &s.Spec.Template.Spec.Containers[0].VolumeMounts
			(*m)[len(*m)-1].MountPath = "/other"
		},
		"writable mount": func(s *appsv1.StatefulSet) {
			m := &s.Spec.Template.Spec.Containers[0].VolumeMounts
			(*m)[len(*m)-1].ReadOnly = false
		},
		"init container token mount": func(s *appsv1.StatefulSet) {
			s.Spec.Template.Spec.InitContainers[0].VolumeMounts = append(s.Spec.Template.Spec.InitContainers[0].VolumeMounts, corev1.VolumeMount{
				Name: "kube-api-access", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true,
			})
		},
		"init container security context": func(s *appsv1.StatefulSet) {
			s.Spec.Template.Spec.InitContainers[0].SecurityContext.RunAsNonRoot = ptr.To(false)
		},
		"other serviceaccount": func(s *appsv1.StatefulSet) { s.Spec.Template.Spec.ServiceAccountName = "unrelated-serviceaccount" },
		"automatic rollout": func(s *appsv1.StatefulSet) {
			s.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.RollingUpdateStatefulSetStrategyType, RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: ptr.To(int32(0))}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			// Restore the restricted fixture as the administrator, then exercise the
			// controller exception with one unauthorized change alongside the handoff.
			reset := original.DeepCopy()
			reset.ResourceVersion = prepared.ResourceVersion
			if reset.Annotations == nil {
				reset.Annotations = map[string]string{}
			}
			reset.Annotations[constants.AnnotationMaintenance] = "true"
			require.NoError(t, k8sClient.Update(ctx, reset))
			changed := prepared.DeepCopy()
			changed.ResourceVersion = reset.ResourceVersion
			mutate(changed)
			requireAdmissionDenied(t, controller.Patch(ctx, changed, client.MergeFrom(reset), client.DryRunAll))
			prepared.ResourceVersion = reset.ResourceVersion
		})
	}
	_, err = manager.PrepareRestoreHandoff(ctx, cluster, original.UID, normal)
	require.NoError(t, err)
	require.NoError(t, controller.Get(ctx, client.ObjectKeyFromObject(cluster), prepared))
	generation := prepared.Generation
	_, err = manager.PrepareRestoreHandoff(ctx, cluster, original.UID, normal)
	require.NoError(t, err)
	repeated := &appsv1.StatefulSet{}
	require.NoError(t, controller.Get(ctx, client.ObjectKeyFromObject(cluster), repeated))
	require.Equal(t, generation, repeated.Generation, "handoff must be idempotent")

	// Ordinary reconciliation must not trigger a second Pod rollout after Resume.
	require.NoError(t, manager.EnsureStatefulSet(ctx, logr.Discard(), managed, normal, spec))
	after := &appsv1.StatefulSet{}
	require.NoError(t, controller.Get(ctx, client.ObjectKeyFromObject(cluster), after))
	require.Equal(t, prepared.Spec.Template, after.Spec.Template)
	require.Equal(t, api.UpdateStrategyRollingUpdate, api.UpdateStrategyType(after.Spec.UpdateStrategy.Type))
}
