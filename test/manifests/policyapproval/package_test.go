package policyapproval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

const chartPath = "../../../charts/openbao-policy-approval"

func TestShippedBundlesMatchBootstrap(t *testing.T) {
	for _, strategy := range []struct {
		name  string
		value api.UpdateStrategyType
	}{{"rolling-update", "RollingUpdate"}, {"blue-green", "BlueGreen"}} {
		for _, backup := range []bool{false, true} {
			cluster := &api.OpenBaoCluster{}
			cluster.Spec.Upgrade = &api.UpgradeConfig{Strategy: strategy.value}
			name := strategy.name
			if backup {
				cluster.Spec.Backup = &api.BackupSchedule{}
				name += "-backup"
			}
			path := filepath.Join(chartPath, "bundles", configbuilder.OperatorPolicyBundleRevision, name+".hcl")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, configbuilder.OperatorPolicyApproval(cluster), string(data),
				"publish a new bundle revision when policy contents change")
		}
	}
}

func TestChartApprovalWorkflow(t *testing.T) {
	setup, err := renderChart(t)
	require.NoError(t, err)
	require.NotContains(t, string(setup), "kind: Job", "new clusters bootstrap their own initial approval")
	require.NotContains(t, string(setup), "kind: Role")
	args := []string{"approval.from=v1/rolling-update", "approval.to=v1/blue-green", "gitops=argocd"}
	data, err := renderChart(t, args...)
	require.NoError(t, err)
	job, cm := jobAndConfig(t, data)
	previous, err := os.ReadFile(filepath.Join(chartPath, "bundles/v1/rolling-update.hcl"))
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join(chartPath, "bundles/v1/blue-green.hcl"))
	require.NoError(t, err)
	require.Equal(t, string(want), cm.Data["policy.hcl"], "YAML must preserve the exact approval bytes")
	container := job.Spec.Template.Spec.Containers[0]
	require.Contains(t, container.Args, fmt.Sprintf("--sha256=%x", sha256.Sum256(want)))
	require.Contains(t, container.Args, fmt.Sprintf("--expected-current-sha256=%x", sha256.Sum256(previous)))
	require.Contains(t, container.Args, "--role="+portauth.PolicyApproverName)
	cluster := &api.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "bao"}}
	projection := job.Spec.Template.Spec.Volumes[1].Projected.Sources[0].ServiceAccountToken
	require.Equal(t, portauth.PolicyApproverAudience(cluster), projection.Audience)
	require.Equal(t, "example-policy-approver", job.Spec.Template.Spec.ServiceAccountName)
	require.False(t, *job.Spec.Template.Spec.AutomountServiceAccountToken)
	require.Nil(t, job.Spec.TTLSecondsAfterFinished)
	require.Equal(t, "Sync", job.Annotations["argocd.argoproj.io/hook"])
	require.Equal(t, "-2", cm.Annotations["argocd.argoproj.io/sync-wave"])
	for _, change := range []string{"approval.to=v1/blue-green-backup", "approval.retry=retry-1"} {
		changed, err := renderChart(t, append(append([]string{}, args...), change)...)
		require.NoError(t, err)
		newJob, _ := jobAndConfig(t, changed)
		require.NotEqual(t, job.Name, newJob.Name, "changed requests must create a new Job")
	}
	for _, invalid := range [][]string{
		{"approval.to=v1/blue-green"},
		{"approval.from=v1/rolling-update", "approval.to=unknown"},
		{"approval.from=v1/rolling-update", "approval.to=v1/blue-green", "image=operator:latest"},
		{"cluster.namespace=openbao-admin"},
	} {
		_, err := renderChart(t, invalid...)
		require.Error(t, err)
	}
	data, err = renderChart(t, "approval.from=absent", "approval.to=v1/blue-green")
	require.NoError(t, err)
	job, _ = jobAndConfig(t, data)
	require.Contains(t, job.Spec.Template.Spec.Containers[0].Args, "--expected-current-sha256=absent")
	require.Empty(t, job.Annotations, "plain mode must remain an ordinary Job for Flux")
}

func renderChart(t *testing.T, values ...string) ([]byte, error) {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is required; make test-policy-approval-chart enforces this in chart CI")
	}
	args := make([]string, 0, 7+2*len(values))
	args = append(args, "template", "example-policy", chartPath, "--namespace", "openbao-admin", "--set",
		"cluster.name=example,cluster.namespace=bao,image=example.invalid/operator@sha256:"+strings.Repeat("0", 64))
	for _, value := range values {
		args = append(args, "--set-string", value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, helm, args...).CombinedOutput()
}

func jobAndConfig(t *testing.T, manifests []byte) (*batchv1.Job, *corev1.ConfigMap) {
	t.Helper()
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(manifests), 4096)
	var job batchv1.Job
	var cm corev1.ConfigMap
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		var meta metav1.TypeMeta
		require.NoError(t, json.Unmarshal(raw, &meta))
		switch meta.Kind {
		case "Job":
			require.NoError(t, json.Unmarshal(raw, &job))
		case "ConfigMap":
			require.NoError(t, json.Unmarshal(raw, &cm))
		}
	}
	require.NotEmpty(t, job.Name)
	require.NotEmpty(t, cm.Name)
	return &job, &cm
}
