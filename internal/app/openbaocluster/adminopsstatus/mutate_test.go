package adminopsstatus

import (
	"context"
	"errors"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestMutateWithReader_SyncsCallerOnlyAfterSuccessfulReadBack(t *testing.T) {
	t.Parallel()
	for _, readFails := range []bool{false, true} {
		name := "successful read-back updates only owned fields and resource version"
		if readFails {
			name = "failed read-back leaves caller unchanged"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scheme := runtime.NewScheme()
			if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			stored := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "readback", Namespace: "default", ResourceVersion: "1"},
				Status:     openbaov1alpha1.OpenBaoClusterStatus{CurrentVersion: "2.5.0"},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(stored).WithObjects(stored).Build()
			readErr := errors.New("read-back unavailable")
			reads := 0
			reader := interceptor.NewClient(c, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					reads++
					if reads == 2 && readFails {
						return readErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})
			cluster := stored.DeepCopy()
			cluster.Status.CurrentVersion = "2.4.4"
			before := cluster.DeepCopy()
			err := MutateWithReader(context.Background(), reader, c, cluster,
				func(obj *openbaov1alpha1.OpenBaoCluster) error {
					obj.Status.Backup = &openbaov1alpha1.BackupStatus{LastFailureReason: "persist-me"}
					return nil
				}, MutateOptions{})

			persisted := &openbaov1alpha1.OpenBaoCluster{}
			if getErr := c.Get(context.Background(), client.ObjectKeyFromObject(cluster), persisted); getErr != nil {
				t.Fatal(getErr)
			}
			if persisted.Status.Backup == nil || persisted.Status.Backup.LastFailureReason != "persist-me" {
				t.Fatalf("persisted backup = %+v, want successful apply", persisted.Status.Backup)
			}
			want := before.DeepCopy()
			if readFails {
				if !errors.Is(err, readErr) {
					t.Fatalf("error = %v, want read-back error", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want.ResourceVersion = persisted.ResourceVersion
				want.Status.Backup = persisted.Status.Backup
			}
			if !reflect.DeepEqual(cluster, want) {
				t.Fatalf("caller = %+v, want %+v", cluster, want)
			}
		})
	}
}
