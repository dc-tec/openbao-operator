package statusops

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestGatherState_CollectsDataPVCStorageClasses(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	className := "gp3"
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
	}
	dataPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-example-0",
			Namespace: "default",
			Labels: map[string]string{
				"openbao.org/cluster": "example",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &className,
		},
	}
	otherPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scratch-volume",
			Namespace: "default",
			Labels: map[string]string{
				"openbao.org/cluster": "example",
			},
		},
	}

	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, dataPVC, otherPVC).Build()
	state, err := GatherState(context.Background(), logr.Discard(), reader, cluster, LabelConfig{
		AppInstanceKey:       "app.kubernetes.io/instance",
		AppManagedByKey:      "app.kubernetes.io/managed-by",
		AppManagedByValue:    "openbao-operator",
		OpenBaoClusterKey:    "openbao.org/cluster",
		OpenBaoComponentKey:  "openbao.org/component",
		BackupComponentValue: "backup",
		AppNameKey:           "app.kubernetes.io/name",
		AppNameValue:         "openbao",
		OpenBaoRevisionKey:   "openbao.org/revision",
	})
	require.NoError(t, err)
	require.Equal(t, 1, state.DataPVCCount)
	require.Equal(t, []string{"gp3"}, state.DataPVCStorageClassNames)
	require.False(t, state.DataPVCStorageClassUnset)
}
