package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
	restoremanager "github.com/dc-tec/openbao-operator/internal/service/restore"
)

type restoreConnection struct {
	config *backupconfig.ExecutorConfig
	client client.Client
	member openbaov1alpha1.RestoreSubmissionClaim
}

func (r *restoreConnection) connect(ctx context.Context) (portopenbao.ClusterActions, func(), error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("configure restore claim client: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	r.client, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("create restore claim client: %w", err)
	}

	address, err := findRestoreLeader(ctx, r.config)
	if err != nil {
		return nil, nil, err
	}

	parsed, err := url.Parse(address)
	if err != nil {
		return nil, nil, err
	}

	name := strings.Split(parsed.Hostname(), ".")[0]
	member, err := r.readMember(ctx, name)
	if err != nil {
		return nil, nil, err
	}

	r.member = member
	address = "https://" + net.JoinHostPort(member.TargetPodIP, "8200")
	token, err := authenticate(ctx, r.config, address)
	if err != nil {
		return nil, nil, err
	}

	return openClusterClient(r.config, "restore", address, token)
}

func (r *restoreConnection) readMember(
	ctx context.Context, name string,
) (openbaov1alpha1.RestoreSubmissionClaim, error) {
	member := openbaov1alpha1.RestoreSubmissionClaim{}
	pod := &corev1.Pod{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: r.config.ClusterNamespace, Name: name}, pod); err != nil {
		return member, err
	}

	owner := metav1.GetControllerOf(pod)
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || owner == nil ||
		owner.Kind != "StatefulSet" || owner.Name != r.config.StatefulSetName || net.ParseIP(pod.Status.PodIP) == nil {
		return member, fmt.Errorf("restore requires the current running target member")
	}

	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "openbao" && status.State.Running != nil && status.ContainerID != "" {
			return openbaov1alpha1.RestoreSubmissionClaim{TargetPodName: name, TargetPodUID: pod.UID,
				TargetPodIP: pod.Status.PodIP, TargetContainerID: status.ContainerID}, nil
		}
	}

	return member, fmt.Errorf("target OpenBao container is not running")
}

func (r *restoreConnection) claim(
	ctx context.Context, bao portopenbao.ClusterActions, digest string, size int64,
) error {
	key := client.ObjectKey{Namespace: r.config.ClusterNamespace, Name: os.Getenv("RESTORE_REQUEST_NAME")}
	uid := types.UID(os.Getenv("RESTORE_REQUEST_UID"))
	pod := &corev1.Pod{}
	if key.Name == "" || uid == "" {
		return fmt.Errorf("restore request identity is required")
	}

	if err := r.client.Get(ctx, client.ObjectKey{Namespace: key.Namespace, Name: os.Getenv("POD_NAME")}, pod); err != nil {
		return err
	}

	owner := metav1.GetControllerOf(pod)
	if pod.DeletionTimestamp != nil || pod.UID == "" || string(pod.UID) != os.Getenv("POD_UID") ||
		owner == nil || owner.Kind != "Job" {
		return fmt.Errorf("restore executor requires its original Job-owned Pod")
	}

	// The Job may start before the controller persists its UID. Only reads wait;
	// the conditional claim is attempted once, without conflict retries.
	for {
		request := &openbaov1alpha1.OpenBaoRestore{}
		if err := r.client.Get(ctx, key, request); err != nil {
			return err
		}
		if request.UID != uid || request.Status.Execution == nil || request.DeletionTimestamp != nil ||
			request.Status.Phase != openbaov1alpha1.RestorePhaseRunning {
			return fmt.Errorf("restore execution changed")
		}
		if request.Status.Execution.JobUID != "" {
			if request.Status.Execution.JobUID != owner.UID {
				return fmt.Errorf("restore executor Job identity differs")
			}
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}

	leader, err := bao.IsLeader(ctx)
	if err != nil {
		return fmt.Errorf("check restore target leadership: %w", err)
	}

	if !leader {
		return fmt.Errorf("restore target is not the observed leader")
	}

	current, err := r.readMember(ctx, r.member.TargetPodName)
	if err != nil {
		return err
	}

	if current != r.member {
		return fmt.Errorf("restore target process changed before submission")
	}

	claim := r.member
	claim.PodUID, claim.Digest, claim.Size = pod.UID, digest, size
	return restoremanager.ClaimRestore(ctx, r.client, key, uid, claim)
}
