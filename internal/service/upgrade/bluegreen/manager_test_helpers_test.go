package bluegreen

import (
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/port/workload"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

const deploymentNameSuffix = "green"

func managedBlueGreenJob(
	job *batchv1.Job,
	cluster *openbaov1alpha1.OpenBaoCluster,
) *batchv1.Job {
	controller := true
	job.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: openbaov1alpha1.GroupVersion.String(),
		Kind:       "OpenBaoCluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: &controller,
	}}
	if job.Annotations == nil {
		job.Annotations = map[string]string{}
	}
	job.Annotations[constants.AnnotationOpenBaoOwnerUID] = string(cluster.UID)
	return job
}

func newManagerWithClientFactory(
	c client.Client,
	scheme *runtime.Scheme,
	workloadRuntime workload.BlueGreenRuntime,
	backupRuntime backup.PreUpgradeSnapshotRuntime,
	clientFactory raftops.OpenBaoClientFactory,
	clientConfig openbao.ClientConfig,
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	mgr := NewManager(c, scheme, workloadRuntime, backupRuntime, clientConfig, imageVerifier, operatorImageVerifier, platform, recorder...)
	if clientFactory != nil {
		mgr.clientFactory = clientFactory
	}
	mgr.clusterOps = newOpenBaoClusterOps(c, mgr.clientFactory)
	return mgr
}
