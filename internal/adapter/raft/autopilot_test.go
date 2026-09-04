package raft

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const OpenBaoClusterNamespace = "openbaocluster-dev"

func TestAutopilotBaseURL_UsesPublicServiceWhenRendered(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = "openbaocluster-dev"
	cluster.Namespace = OpenBaoClusterNamespace
	cluster.Spec.Service = &openbaov1alpha1.ServiceConfig{}

	got := autopilotBaseURL(cluster)
	want := "https://openbaocluster-dev-public.openbaocluster-dev.svc:8200"
	if got != want {
		t.Fatalf("autopilotBaseURL() = %q, want %q", got, want)
	}
}

func TestAutopilotBaseURL_FallsBackToHeadlessService(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = "openbaocluster-dev"
	cluster.Namespace = OpenBaoClusterNamespace

	got := autopilotBaseURL(cluster)
	want := "https://openbaocluster-dev.openbaocluster-dev.svc:8200"
	if got != want {
		t.Fatalf("autopilotBaseURL() = %q, want %q", got, want)
	}
}
