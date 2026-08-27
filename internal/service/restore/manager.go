// Package restore provides restore management for OpenBao clusters.
// It handles restoring snapshots from object storage to an OpenBao cluster.
package restore

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type adminOpsStatusMutator func(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	mutate func(obj *openbaov1alpha1.OpenBaoCluster) error,
	forceOwnership bool,
) error

const (
	// RestoreJobNamePrefix is the prefix for restore job names.
	RestoreJobNamePrefix = constants.PrefixRestoreJob
	// RestoreServiceAccountSuffix is appended to cluster name for the restore SA.
	RestoreServiceAccountSuffix = constants.SuffixRestoreServiceAccount
	// RestoreConditionType is the condition type for restore operations.
	RestoreConditionType = constants.RestoreConditionType // This will need to be added to conditions.go if missed

	restoreRequeueImmediately = 1 * time.Second
	restoreRequeueJobPoll     = 15 * time.Second
	restoreRequeueJobCheck    = 10 * time.Second
)

// Manager orchestrates restore operations for OpenBao clusters.
type Manager struct {
	client                client.Client
	reader                client.Reader
	scheme                *runtime.Scheme
	recorder              events.EventRecorder
	operatorImageVerifier imageverify.Verifier
	adminOpsMutator       adminOpsStatusMutator
	clientConfig          portopenbao.ClientConfig
	Platform              string
}

// WithAdminOpsStatusMutator configures the adminops-plane status persistence hook.
func (m *Manager) WithAdminOpsStatusMutator(mutator adminOpsStatusMutator) *Manager {
	if mutator != nil {
		m.adminOpsMutator = mutator
	}
	return m
}

// NewManager creates a new restore Manager.
func NewManager(
	c client.Client,
	scheme *runtime.Scheme,
	recorder events.EventRecorder,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	clientConfigs ...portopenbao.ClientConfig,
) *Manager {
	clientConfig := portopenbao.ClientConfig{}
	if len(clientConfigs) > 0 {
		clientConfig = clientConfigs[0]
	}

	return &Manager{
		client:                c,
		reader:                c,
		scheme:                scheme,
		recorder:              recorder,
		operatorImageVerifier: operatorImageVerifier,
		clientConfig:          clientConfig,
		Platform:              platform,
	}
}

// WithReader configures a live reader for lock/status read-before-write flows.
func (m *Manager) WithReader(reader client.Reader) *Manager {
	if reader != nil {
		m.reader = reader
	}
	return m
}
