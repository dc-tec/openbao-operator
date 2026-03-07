package backup

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

type metricsBlobStore struct {
	headInfo   *blobstore.ObjectInfo
	headErr    error
	closeCount int
}

type capturingLogSink struct {
	infoCount  int
	errorCount int
}

func (c *capturingLogSink) Init(logr.RuntimeInfo) {}

func (c *capturingLogSink) Enabled(int) bool { return true }

func (c *capturingLogSink) Info(int, string, ...interface{}) {
	c.infoCount++
}

func (c *capturingLogSink) Error(error, string, ...interface{}) {
	c.errorCount++
}

func (c *capturingLogSink) WithValues(...interface{}) logr.LogSink { return c }

func (c *capturingLogSink) WithName(string) logr.LogSink { return c }

func (m *metricsBlobStore) Upload(context.Context, string, io.Reader) error { return nil }

func (m *metricsBlobStore) Download(context.Context, string) (io.ReadCloser, error) { return nil, nil }

func (m *metricsBlobStore) Delete(context.Context, string) error { return nil }

func (m *metricsBlobStore) DeleteBatch(context.Context, []string) error { return nil }

func (m *metricsBlobStore) List(context.Context, string) ([]blobstore.ObjectInfo, error) {
	return nil, nil
}

func (m *metricsBlobStore) Head(context.Context, string) (*blobstore.ObjectInfo, error) {
	return m.headInfo, m.headErr
}

func (m *metricsBlobStore) Close() error {
	m.closeCount++
	return nil
}

func resetBackupTestState(namespace, name string) {
	NewMetrics(namespace, name).Clear()
	backupJobMetricsSeen = sync.Map{}
}

const (
	testKeepAnnotationKey   = "openbao.org/keep"
	testKeepAnnotationValue = "preserved"
)

func newBackupManager(k8sClient client.Client) *Manager {
	return &Manager{
		client: k8sClient,
		scheme: testScheme,
	}
}

func newBackupJobForCluster(cluster *openbaov1alpha1.OpenBaoCluster, name string, createdAt time.Time) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         cluster.Namespace,
			UID:               types.UID(name),
			CreationTimestamp: metav1.NewTime(createdAt),
			Labels:            backupLabels(cluster),
		},
	}
}

func ptrToTime(value time.Time) *metav1.Time {
	metaTime := metav1.NewTime(value)
	return &metaTime
}
