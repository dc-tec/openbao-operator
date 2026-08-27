package bluegreen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

const (
	jobNamePrefix = "upgrade-"
)

type JobResult = upgrade.JobResult

type buildJobFunc func(jobName string) (*batchv1.Job, error)

func ensureJob(
	ctx context.Context,
	c client.Client,
	reader client.Reader,
	scheme *runtime.Scheme,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	build buildJobFunc,
	createLogKeysAndValues ...any,
) (*JobResult, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if reader == nil {
		reader = c
	}
	if jobName == "" {
		return nil, fmt.Errorf("jobName is required")
	}
	if build == nil {
		return nil, fmt.Errorf("build function is required")
	}

	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}
	job, err := opslifecycle.ReadManagedJob(
		ctx,
		reader,
		jobKey,
		cluster,
		openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
		"observe blue-green upgrade",
	)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		built, err := build(jobName)
		if err != nil {
			return nil, err
		}

		if err := opslifecycle.PrepareManagedJobOwner(built, cluster, scheme); err != nil {
			return nil, fmt.Errorf("failed to prepare Job ownership %s/%s: %w", cluster.Namespace, jobName, err)
		}

		logger.Info("Creating Job", append([]any{"job", jobName}, createLogKeysAndValues...)...)
		if err := c.Create(ctx, built); err != nil {
			if apierrors.IsAlreadyExists(err) {
				existing, readErr := opslifecycle.ReadManagedJob(
					ctx,
					reader,
					jobKey,
					cluster,
					openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
					"use existing blue-green upgrade",
				)
				if readErr != nil {
					return nil, fmt.Errorf("job create collided with an untrusted existing Job: %w", readErr)
				}
				logger.V(1).Info("Job already exists after create attempt; ownership verified", "job", jobName)
				return jobResultFromJob(existing), nil
			}
			return nil, fmt.Errorf("failed to create Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		return &JobResult{
			Name:    jobName,
			Exists:  true,
			Running: true,
		}, nil
	}

	return jobResultFromJob(job), nil
}

func jobResultFromJob(job *batchv1.Job) *JobResult {
	jobName := job.Name
	if kube.JobSucceeded(job) {
		return &JobResult{
			Name:      jobName,
			Exists:    true,
			Succeeded: true,
		}
	}

	if kube.JobFailed(job) {
		return &JobResult{
			Name:   jobName,
			Exists: true,
			Failed: true,
		}
	}

	return &JobResult{
		Name:    jobName,
		Exists:  true,
		Running: true,
	}
}

func getJobStatus(ctx context.Context, reader client.Reader, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (*JobResult, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if jobName == "" {
		return nil, fmt.Errorf("jobName is required")
	}

	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}
	job, err := opslifecycle.ReadManagedJob(
		ctx,
		reader,
		jobKey,
		cluster,
		openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
		"observe blue-green upgrade",
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &JobResult{Name: jobName, Exists: false}, nil
		}
		return nil, fmt.Errorf("failed to get Job %s/%s: %w", cluster.Namespace, jobName, err)
	}

	return jobResultFromJob(job), nil
}

func preUpgradeSnapshotJobName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	blueRevision := ""
	if cluster.Status.BlueGreen != nil {
		blueRevision = cluster.Status.BlueGreen.BlueRevision
	}

	payload := fmt.Sprintf("%s|pre-upgrade-snapshot|%s|%s|%s|%d|%s",
		cluster.Name,
		cluster.Status.CurrentVersion,
		cluster.Spec.Version,
		cluster.Spec.Image,
		cluster.Spec.Replicas,
		blueRevision,
	)
	sum := sha256.Sum256([]byte(payload))
	suffix := hex.EncodeToString(sum[:])[:10]

	base := fmt.Sprintf("%s%s-preupgrade-snapshot", jobNamePrefix, cluster.Name)
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, "_", "-")

	maxBaseLen := 63 - 1 - len(suffix) // "-" + suffix
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
		base = strings.TrimRight(base, "-")
	}

	return fmt.Sprintf("%s-%s", base, suffix)
}
