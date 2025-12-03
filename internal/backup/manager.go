// Package backup provides backup management for OpenBao clusters.
// It handles scheduled snapshots to object storage and retention policy enforcement.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/openbao"
	"github.com/openbao/operator/internal/storage"
)

const (
	// tlsCASecretKey is the key for the CA certificate in the TLS Secret.
	tlsCASecretKey = "ca.crt"
	// backupServiceAccountSuffix is the suffix for backup ServiceAccount names.
	backupServiceAccountSuffix = "-backup-serviceaccount"
)

// ErrNoBackupToken indicates that no suitable backup token is configured for
// the cluster. This occurs when neither Kubernetes Auth role nor backup token Secret
// is provided, or the referenced Secret is missing.
var ErrNoBackupToken = errors.New("no backup token configured: either kubernetesAuthRole or tokenSecretRef must be set")

// Manager reconciles backup configuration and execution for an OpenBaoCluster.
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: scheme,
	}
}

// Reconcile ensures backup configuration and status are aligned with the desired state for the given OpenBaoCluster.
// It checks if a backup is due, executes it if needed, and applies retention policies.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Skip if backup is not configured
	if cluster.Spec.Backup == nil {
		return nil
	}

	logger = logger.WithValues("component", "backup")
	metrics := NewMetrics(cluster.Namespace, cluster.Name)

	// Ensure backup ServiceAccount exists (for Kubernetes Auth)
	if err := m.ensureBackupServiceAccount(ctx, logger, cluster); err != nil {
		return fmt.Errorf("failed to ensure backup ServiceAccount: %w", err)
	}

	// Initialize backup status if needed
	if cluster.Status.Backup == nil {
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	}

	// Pre-flight checks
	if err := m.checkPreconditions(ctx, logger, cluster); err != nil {
		if errors.Is(err, ErrNoBackupToken) {
			m.setBackingUpCondition(cluster, false, "NoBackupToken", err.Error())
		}

		logger.V(1).Info("Backup preconditions not met", "reason", err.Error())
		return nil // Don't return error - preconditions not met is not a reconcile failure
	}

	// Check if backup is due
	isDue, err := m.isBackupDue(cluster)
	if err != nil {
		return fmt.Errorf("failed to check backup schedule: %w", err)
	}

	if !isDue {
		// Update next scheduled backup time
		if err := m.updateNextScheduled(cluster); err != nil {
			logger.Error(err, "Failed to update next scheduled backup time")
		}
		return nil
	}

	logger.Info("Backup is due, creating backup Job")
	metrics.SetInProgress(true)

	// Create or check backup Job
	jobInProgress, err := m.ensureBackupJob(ctx, logger, cluster)
	if err != nil {
		return fmt.Errorf("failed to ensure backup Job: %w", err)
	}

	if jobInProgress {
		// Job is running - process its status
		if err := m.processBackupJobResult(ctx, logger, cluster); err != nil {
			return fmt.Errorf("failed to process backup Job result: %w", err)
		}
		// Requeue to check Job status again
		return nil
	}

	// Check if there's a completed Job to process
	if err := m.processBackupJobResult(ctx, logger, cluster); err != nil {
		return fmt.Errorf("failed to process backup Job result: %w", err)
	}

	// If backup completed successfully, apply retention and update schedule
	if cluster.Status.Backup != nil && cluster.Status.Backup.LastBackupTime != nil {
		// Apply retention policy
		if cluster.Spec.Backup.Retention != nil {
			if err := m.applyRetention(ctx, logger, cluster, metrics); err != nil {
				// Log but don't fail - retention errors shouldn't fail the backup
				logger.Error(err, "Failed to apply retention policy")
			}
		}

		// Update next scheduled backup time
		if err := m.updateNextScheduled(cluster); err != nil {
			logger.Error(err, "Failed to update next scheduled backup time")
		}
	}

	return nil
}

// BackupResult contains the result of a successful backup.
type BackupResult struct {
	// Key is the object storage key where the backup was stored.
	Key string
	// Size is the size of the backup in bytes.
	Size int64
}

// checkPreconditions verifies that backup can proceed.
func (m *Manager) checkPreconditions(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Check cluster is initialized
	if !cluster.Status.Initialized {
		return fmt.Errorf("cluster is not initialized")
	}

	// Check cluster phase - don't backup during initialization
	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseInitializing {
		return fmt.Errorf("cluster is initializing")
	}

	// Check if upgrade is in progress - skip scheduled backups during upgrades
	// Exception: Pre-upgrade backups are triggered by the upgrade manager, not here
	if cluster.Status.Upgrade != nil {
		return fmt.Errorf("upgrade in progress")
	}

	// Check if another backup is in progress
	backingUpCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackingUp))
	if backingUpCond != nil && backingUpCond.Status == metav1.ConditionTrue {
		return fmt.Errorf("backup already in progress")
	}

	// Check we have a token for backup.
	// All clusters (both standard and self-init) must use either Kubernetes Auth
	// or a backup token Secret. Root tokens are not used for security reasons.
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return ErrNoBackupToken
	}

	// Check if Kubernetes Auth is configured (preferred method)
	hasKubernetesAuth := strings.TrimSpace(backupCfg.KubernetesAuthRole) != ""

	// Check if static token is configured (fallback method)
	hasTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""

	// At least one authentication method must be configured
	if !hasKubernetesAuth && !hasTokenSecret {
		return ErrNoBackupToken
	}

	// If using token secret, verify it exists
	if hasTokenSecret {
		secretNamespace := cluster.Namespace
		if ns := strings.TrimSpace(backupCfg.TokenSecretRef.Namespace); ns != "" {
			secretNamespace = ns
		}

		secretName := types.NamespacedName{
			Namespace: secretNamespace,
			Name:      backupCfg.TokenSecretRef.Name,
		}

		secret := &corev1.Secret{}
		if err := m.client.Get(ctx, secretName, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("backup token Secret %s/%s not found: %w", secretNamespace, backupCfg.TokenSecretRef.Name, ErrNoBackupToken)
			}
			return fmt.Errorf("failed to get backup token Secret %s/%s: %w", secretNamespace, backupCfg.TokenSecretRef.Name, err)
		}
	}

	return nil
}

// isBackupDue checks if a backup should be executed now.
func (m *Manager) isBackupDue(cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	schedule := cluster.Spec.Backup.Schedule

	var lastBackup time.Time
	if cluster.Status.Backup != nil && cluster.Status.Backup.LastBackupTime != nil {
		lastBackup = cluster.Status.Backup.LastBackupTime.Time
	}

	return IsDue(schedule, lastBackup, time.Now().UTC())
}

// updateNextScheduled calculates and sets the next scheduled backup time.
func (m *Manager) updateNextScheduled(cluster *openbaov1alpha1.OpenBaoCluster) error {
	var lastBackup time.Time
	if cluster.Status.Backup != nil && cluster.Status.Backup.LastBackupTime != nil {
		lastBackup = cluster.Status.Backup.LastBackupTime.Time
	}

	nextBackup, err := CalculateNextBackup(cluster.Spec.Backup.Schedule, lastBackup)
	if err != nil {
		return err
	}

	nextBackupMeta := metav1.NewTime(nextBackup)
	cluster.Status.Backup.NextScheduledBackup = &nextBackupMeta
	return nil
}

// executeBackup performs the actual backup operation.
func (m *Manager) executeBackup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*BackupResult, error) {
	// Set backing up condition
	m.setBackingUpCondition(cluster, true, "BackupInProgress", "Backup is currently in progress")

	// Load storage credentials
	creds, err := storage.LoadCredentials(ctx, m.client, cluster.Spec.Backup.Target.CredentialsSecretRef, cluster.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to load storage credentials: %w", err)
	}

	// Create storage client
	usePathStyle := cluster.Spec.Backup.Target.UsePathStyle
	storageClient, err := storage.NewS3ClientFromCredentials(
		ctx,
		cluster.Spec.Backup.Target.Endpoint,
		cluster.Spec.Backup.Target.Bucket,
		creds,
		usePathStyle,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	// Get OpenBao token
	token, err := m.getOpenBaoToken(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenBao token: %w", err)
	}

	// Get CA certificate for TLS
	caCert, err := m.getTLSCA(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS CA: %w", err)
	}

	// Find leader pod
	leaderURL, err := m.findLeader(ctx, logger, cluster, token, caCert)
	if err != nil {
		return nil, fmt.Errorf("failed to find leader: %w", err)
	}

	logger.Info("Found leader for backup", "leaderURL", leaderURL)

	// Create OpenBao client for the leader
	baoClient, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: leaderURL,
		Token:   token,
		CACert:  caCert,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Generate backup key
	backupKey, err := GenerateBackupKey(
		cluster.Spec.Backup.Target.PathPrefix,
		cluster.Namespace,
		cluster.Name,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup key: %w", err)
	}

	logger.Info("Starting snapshot stream", "key", backupKey)

	// Stream snapshot from OpenBao to object storage
	snapshotResp, err := baoClient.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot from OpenBao: %w", err)
	}
	defer func() {
		_ = snapshotResp.Body.Close()
	}()

	// Create a counting reader to track bytes uploaded
	countReader := &countingReader{reader: snapshotResp.Body}

	// Upload to object storage
	if err := storageClient.Upload(ctx, backupKey, countReader, snapshotResp.ContentLength); err != nil {
		return nil, fmt.Errorf("failed to upload backup to storage: %w", err)
	}

	// Verify upload
	objInfo, err := storageClient.Head(ctx, backupKey)
	if err != nil {
		return nil, fmt.Errorf("failed to verify backup upload: %w", err)
	}
	if objInfo == nil {
		return nil, fmt.Errorf("backup verification failed: object not found after upload")
	}

	logger.Info("Backup uploaded and verified", "key", backupKey, "size", objInfo.Size)

	return &BackupResult{
		Key:  backupKey,
		Size: objInfo.Size,
	}, nil
}

// getOpenBaoToken retrieves the authentication token for OpenBao API calls.
// This function is only called when using static token authentication (not Kubernetes Auth).
func (m *Manager) getOpenBaoToken(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil || backupCfg.TokenSecretRef == nil || strings.TrimSpace(backupCfg.TokenSecretRef.Name) == "" {
		return "", ErrNoBackupToken
	}

	secretNamespace := cluster.Namespace
	if ns := strings.TrimSpace(backupCfg.TokenSecretRef.Namespace); ns != "" {
		secretNamespace = ns
	}

	secretName := types.NamespacedName{
		Namespace: secretNamespace,
		Name:      backupCfg.TokenSecretRef.Name,
	}

	secret := &corev1.Secret{}
	if err := m.client.Get(ctx, secretName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("backup token Secret %s/%s not found: %w", secretNamespace, backupCfg.TokenSecretRef.Name, ErrNoBackupToken)
		}
		return "", fmt.Errorf("failed to get backup token Secret %s/%s: %w", secretNamespace, backupCfg.TokenSecretRef.Name, err)
	}

	// Extract token from secret (default key is "token")
	tokenKey := "token"
	if secret.Data == nil {
		return "", fmt.Errorf("backup token Secret %s/%s has no data", secretNamespace, backupCfg.TokenSecretRef.Name)
	}

	token, ok := secret.Data[tokenKey]
	if !ok || len(token) == 0 {
		return "", fmt.Errorf("backup token Secret %s/%s missing %q key", secretNamespace, backupCfg.TokenSecretRef.Name, tokenKey)
	}

	return strings.TrimSpace(string(token)), nil
}

// getTLSCA retrieves the CA certificate for OpenBao TLS verification.
func (m *Manager) getTLSCA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	tlsSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      fmt.Sprintf("%s-tls-ca", cluster.Name),
	}
	if err := m.client.Get(ctx, secretName, tlsSecret); err != nil {
		return nil, fmt.Errorf("failed to get TLS CA Secret: %w", err)
	}

	caCert, ok := tlsSecret.Data[tlsCASecretKey]
	if !ok {
		return nil, fmt.Errorf("TLS CA Secret missing '%s' key", tlsCASecretKey)
	}

	return caCert, nil
}

// findLeader discovers the current Raft leader by querying health endpoints.
func (m *Manager) findLeader(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, token string, caCert []byte) (string, error) {
	// Query all pods to find the leader
	for i := int32(0); i < cluster.Spec.Replicas; i++ {
		podName := fmt.Sprintf("%s-%d", cluster.Name, i)
		podURL := fmt.Sprintf("https://%s.%s.%s.svc:8200", podName, cluster.Name, cluster.Namespace)

		baoClient, err := openbao.NewClient(openbao.ClientConfig{
			BaseURL: podURL,
			Token:   token,
			CACert:  caCert,
		})
		if err != nil {
			logger.V(1).Info("Failed to create client for pod", "pod", podName, "error", err)
			continue
		}

		isLeader, err := baoClient.IsLeader(ctx)
		if err != nil {
			logger.V(1).Info("Failed to check leader status for pod", "pod", podName, "error", err)
			continue
		}

		if isLeader {
			return podURL, nil
		}
	}

	return "", fmt.Errorf("no leader found among %d pods", cluster.Spec.Replicas)
}

// applyRetention applies the retention policy after a successful backup.
func (m *Manager) applyRetention(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *Metrics) error {
	retention := cluster.Spec.Backup.Retention
	if retention == nil {
		return nil
	}

	// Parse MaxAge duration
	maxAge, err := ParseRetentionMaxAge(retention.MaxAge)
	if err != nil {
		return fmt.Errorf("failed to parse retention maxAge: %w", err)
	}

	policy := RetentionPolicy{
		MaxCount: retention.MaxCount,
		MaxAge:   maxAge,
	}

	// Load storage credentials
	creds, err := storage.LoadCredentials(ctx, m.client, cluster.Spec.Backup.Target.CredentialsSecretRef, cluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to load storage credentials for retention: %w", err)
	}

	// Create storage client
	usePathStyle := cluster.Spec.Backup.Target.UsePathStyle
	storageClient, err := storage.NewS3ClientFromCredentials(
		ctx,
		cluster.Spec.Backup.Target.Endpoint,
		cluster.Spec.Backup.Target.Bucket,
		creds,
		usePathStyle,
	)
	if err != nil {
		return fmt.Errorf("failed to create storage client for retention: %w", err)
	}

	// Get backup list prefix
	prefix := GetBackupListPrefix(
		cluster.Spec.Backup.Target.PathPrefix,
		cluster.Namespace,
		cluster.Name,
	)

	result, err := ApplyRetention(ctx, logger, storageClient, prefix, policy)
	if err != nil {
		return err
	}

	// Record metrics
	totalDeleted := result.DeletedByCount + result.DeletedByAge
	if totalDeleted > 0 {
		metrics.IncrementRetentionDeleted(totalDeleted)
	}

	return nil
}

// ensureBackupServiceAccount creates or updates the ServiceAccount for backup Jobs.
// This ServiceAccount is used for Kubernetes Auth authentication to OpenBao.
func (m *Manager) ensureBackupServiceAccount(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := backupServiceAccountName(cluster)

	sa := &corev1.ServiceAccount{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      saName,
	}, sa)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get backup ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}

		logger.Info("Backup ServiceAccount not found; creating", "serviceaccount", saName)

		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "openbao",
					"app.kubernetes.io/instance":   cluster.Name,
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/cluster":          cluster.Name,
					"openbao.org/component":        "backup",
				},
			},
		}

		// Set OwnerReference for garbage collection
		if err := controllerutil.SetControllerReference(cluster, sa, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on backup ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}

		if err := m.client.Create(ctx, sa); err != nil {
			return fmt.Errorf("failed to create backup ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}

		return nil
	}

	// Update labels if needed
	if sa.Labels == nil {
		sa.Labels = make(map[string]string)
	}
	needsUpdate := false
	expectedLabels := map[string]string{
		"app.kubernetes.io/name":       "openbao",
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "openbao-operator",
		"openbao.org/cluster":          cluster.Name,
		"openbao.org/component":        "backup",
	}
	for k, v := range expectedLabels {
		if sa.Labels[k] != v {
			sa.Labels[k] = v
			needsUpdate = true
		}
	}

	if needsUpdate {
		if err := m.client.Update(ctx, sa); err != nil {
			return fmt.Errorf("failed to update backup ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}
	}

	return nil
}

// backupServiceAccountName returns the name for the backup ServiceAccount.
func backupServiceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + backupServiceAccountSuffix
}

// setBackingUpCondition sets the BackingUp condition on the cluster status.
func (m *Manager) setBackingUpCondition(cluster *openbaov1alpha1.OpenBaoCluster, isBackingUp bool, reason, message string) {
	status := metav1.ConditionFalse
	if isBackingUp {
		status = metav1.ConditionTrue
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionBackingUp),
		Status:             status,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// countingReader wraps an io.Reader to count bytes read.
type countingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}
