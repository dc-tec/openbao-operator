// Package backup provides backup management for OpenBao clusters.
// It handles scheduled snapshots to object storage and retention policy enforcement.
package backup

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

const backupOperationLockHolder = constants.ControllerNameOpenBaoCluster + "/backup"

var backupOperationLock = opslifecycle.OperationLock{
	Holder:    backupOperationLockHolder,
	Operation: openbaov1alpha1.ClusterOperationBackup,
}

// Manager reconciles backup configuration and execution for an OpenBaoCluster.
type Manager struct {
	client                client.Client
	reader                client.Reader
	scheme                *runtime.Scheme
	recorder              events.EventRecorder
	clientConfig          portopenbao.ClientConfig
	operatorImageVerifier imageverify.Verifier
	Platform              string
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme, clientConfig portopenbao.ClientConfig, operatorImageVerifier imageverify.Verifier, platform string, recorder ...events.EventRecorder) *Manager {
	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	return &Manager{
		client:                c,
		reader:                c,
		scheme:                scheme,
		recorder:              eventRecorder,
		clientConfig:          clientConfig,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
	}
}

// WithReader configures a live reader for status read-before-write flows.
func (m *Manager) WithReader(reader client.Reader) *Manager {
	if reader != nil {
		m.reader = reader
	}
	return m
}

// BackupResult contains the result of a successful backup.
type BackupResult struct {
	// Key is the object storage key where the backup was stored.
	Key string
	// Size is the size of the backup in bytes.
	Size int64
}
