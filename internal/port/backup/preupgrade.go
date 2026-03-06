package backup

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// JobBuildOptions configures pre-upgrade snapshot backup job creation.
type JobBuildOptions struct {
	JobName                string
	FilenamePrefix         string
	VerifiedExecutorDigest string
	ClientConfig           portopenbao.ClientConfig
	Platform               string
	TargetStatefulSetName  string
}

// PreUpgradeSnapshotRuntime defines backup operations required by upgrade strategies.
type PreUpgradeSnapshotRuntime interface {
	EnsureServiceAccount(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error
	EnsureRBAC(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error
	BuildPreUpgradeJob(cluster *openbaov1alpha1.OpenBaoCluster, opts JobBuildOptions) (*batchv1.Job, error)
}
