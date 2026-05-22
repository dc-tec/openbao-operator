// Package restore provides restore management for OpenBao clusters.
// It handles restoring snapshots from object storage to an OpenBao cluster.
package restore

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	// RestoreJobNamePrefix is the prefix for restore job names.
	RestoreJobNamePrefix = constants.PrefixRestoreJob
	// RestoreJobTTLSeconds is the TTL for completed/failed restore jobs.
	RestoreJobTTLSeconds = 3600 // 1 hour
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
	clientConfig          portopenbao.ClientConfig
	Platform              string
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
