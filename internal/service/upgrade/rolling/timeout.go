package rolling

import (
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

type upgradeTimeoutSpec struct {
	Deadline time.Duration
	Reason   string
	Message  string
	Error    string
}

func failUpgradeIfStartedTimeout(cluster *openbaov1alpha1.OpenBaoCluster, timeout upgradeTimeoutSpec) error {
	if cluster == nil || cluster.Status.Upgrade == nil {
		return nil
	}
	return failUpgradeIfTimestampTimeout(cluster, cluster.Status.Upgrade.StartedAt, timeout)
}

func failUpgradeIfTimestampTimeout(cluster *openbaov1alpha1.OpenBaoCluster, startedAt *metav1.Time, timeout upgradeTimeoutSpec) error {
	if startedAt == nil {
		return nil
	}
	if time.Since(startedAt.Time) <= timeout.Deadline {
		return nil
	}

	core.SetUpgradeFailed(&cluster.Status, timeout.Reason, timeout.Message)
	return errors.New(timeout.Error)
}

func podRevisionTimeout(podName string) upgradeTimeoutSpec {
	return upgradeTimeoutSpec{
		Deadline: upgrade.DefaultPodReadyTimeout,
		Reason:   upgrade.ReasonPodNotReady,
		Message:  fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout),
		Error:    fmt.Sprintf("pod %s did not roll to update revision within %v", podName, upgrade.DefaultPodReadyTimeout),
	}
}

func podReadyTimeout(podName string) upgradeTimeoutSpec {
	return upgradeTimeoutSpec{
		Deadline: upgrade.DefaultPodReadyTimeout,
		Reason:   upgrade.ReasonPodNotReady,
		Message:  fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout),
		Error:    fmt.Sprintf("pod %s did not become ready within %v", podName, upgrade.DefaultPodReadyTimeout),
	}
}

func podHealthTimeout(podName string) upgradeTimeoutSpec {
	return upgradeTimeoutSpec{
		Deadline: upgrade.DefaultPodReadyTimeout + upgrade.DefaultHealthCheckTimeout,
		Reason:   upgrade.ReasonHealthCheckFailed,
		Message:  fmt.Sprintf(upgrade.MessageHealthCheckFailed, podName, "timeout"),
		Error:    fmt.Sprintf("OpenBao health check timeout for pod %s", podName),
	}
}

func stepDownTimeout(podName string) upgradeTimeoutSpec {
	return upgradeTimeoutSpec{
		Deadline: upgrade.DefaultStepDownTimeout,
		Reason:   upgrade.ReasonStepDownTimeout,
		Message:  fmt.Sprintf(upgrade.MessageStepDownTimeout, podName),
		Error:    fmt.Sprintf("step-down timeout for pod %s: exceeded %v", podName, upgrade.DefaultStepDownTimeout),
	}
}
