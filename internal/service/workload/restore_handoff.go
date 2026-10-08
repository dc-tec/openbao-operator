package workload

import (
	"context"
	"path"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
)

// RestoreHandoffBlockedError reports an operator-defined handoff precondition.
// Its message contains no provider data and can be shown in restore status.
type RestoreHandoffBlockedError struct{ Message string }

func (e *RestoreHandoffBlockedError) Error() string { return e.Message }

// PrepareRestoreHandoff prepares a retained single-voter target for its managed
// Resume restart. The caller keeps the restore hold and controls Pod deletion.
// configContent must be rendered without the restore-origin restriction.
func (m *Manager) PrepareRestoreHandoff(ctx context.Context, cluster *api.OpenBaoCluster, uid types.UID, configContent string) (string, error) {
	if cluster.Annotations[constants.AnnotationRestoreOrigin] == "" ||
		cluster.Annotations[constants.AnnotationRestoreHold] == "" || cluster.Spec.Replicas != 1 ||
		(cluster.Spec.ReadReplicas != nil && cluster.Spec.ReadReplicas.Replicas != 0) {
		return "", &RestoreHandoffBlockedError{Message: "restore handoff requires a held, fresh single-voter target"}
	}

	sts := &appsv1.StatefulSet{}
	if err := m.reader.Get(ctx, client.ObjectKeyFromObject(cluster), sts); err != nil {
		return "", err
	}
	if uid == "" || sts.UID != uid || !metav1.IsControlledBy(sts, cluster) || sts.DeletionTimestamp != nil ||
		sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		return "", &RestoreHandoffBlockedError{Message: "restore handoff requires the recorded target StatefulSet"}
	}

	// Freeze automatic rollouts before changing the configuration. Config is
	// rendered only at Pod initialization; the running Pod keeps its old config.
	if sts.Spec.UpdateStrategy.Type != appsv1.OnDeleteStatefulSetStrategyType {
		before := sts.DeepCopy()
		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType}
		if err := m.client.Patch(ctx, sts, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
			return "", err
		}
	}

	if err := m.ensureConfigMapWithName(ctx, cluster, configMapNameForSpec(cluster, StatefulSetSpec{}), configContent); err != nil {
		return "", err
	}

	before := sts.DeepCopy()
	managed := cluster.DeepCopy()
	delete(managed.Annotations, constants.AnnotationRestoreOrigin)
	pod := &sts.Spec.Template.Spec

	// Reuse the normal builders for the fixed projection and mount. Keep every
	// other admission-locked field, including user-provided seal wiring, intact.
	if !slices.ContainsFunc(pod.Volumes, func(v corev1.Volume) bool { return v.Name == kubeAPIAccessVolumeName }) {
		for _, volume := range buildStatefulSetVolumes(managed, StatefulSetSpec{}) {
			if volume.Name == kubeAPIAccessVolumeName {
				pod.Volumes = append(pod.Volumes, volume)
			}
		}
	}
	container := slices.IndexFunc(pod.Containers, func(c corev1.Container) bool { return c.Name == "openbao" })
	if container < 0 {
		return "", &RestoreHandoffBlockedError{Message: "restore handoff requires the OpenBao container"}
	}
	mounts := &pod.Containers[container].VolumeMounts
	if !slices.ContainsFunc(*mounts, func(v corev1.VolumeMount) bool { return v.Name == kubeAPIAccessVolumeName }) {
		for _, mount := range buildContainerVolumeMounts(managed, path.Dir(openBaoRenderedConfig)) {
			if mount.Name == kubeAPIAccessVolumeName {
				*mounts = append(*mounts, mount)
			}
		}
	}
	pod.EnableServiceLinks = ptr.To(true)
	if sts.Spec.Template.Annotations == nil {
		sts.Spec.Template.Annotations = make(map[string]string)
	}
	hash := computeConfigHash(configContent)
	sts.Spec.Template.Annotations[configHashAnnotation] = hash
	if !apiequality.Semantic.DeepEqual(before.Spec, sts.Spec) {
		if err := m.client.Patch(ctx, sts, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
			return "", err
		}
	}
	return hash, nil
}
