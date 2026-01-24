package raft

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/openbao"
)

func ptrToInt32(v int32) *int32 {
	return &v
}

func ptrTo(v bool) *bool {
	return &v
}

func TestBuildAutopilotConfig(t *testing.T) {
	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    openbao.AutopilotConfig
		wantErr bool
	}{
		{
			name: "Hardened profile with 3 replicas",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					Replicas: 3,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             true,
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      3,
			},
		},
		{
			name: "Hardened profile with 5 replicas",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					Replicas: 5,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             true,
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      5,
			},
		},
		{
			name: "Hardened profile with 1 replica (edge case, should still use 3)",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					Replicas: 1,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             true,
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      3,
			},
		},
		{
			name: "Development profile with 1 replica",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      1,
			},
		},
		{
			name: "Development profile with 2 replicas",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 2,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      2,
			},
		},
		{
			name: "Development profile with 3 replicas",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 3,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             true,
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      3,
			},
		},
		{
			name: "User-provided MinQuorum override (Hardened)",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					Replicas: 3,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								MinQuorum: ptrToInt32(5),
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             true,
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      5, // User override respected
			},
		},
		{
			name: "User-provided MinQuorum override (Development)",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								MinQuorum: ptrToInt32(2),
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      2, // User override respected
			},
		},
		{
			name: "User-provided ServerStabilizationTime override",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								ServerStabilizationTime: "30s",
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "30s", // User override respected
				MinQuorum:                      1,
			},
		},
		{
			name: "User-provided DeadServerLastContactThreshold override",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								DeadServerLastContactThreshold: "1m",
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "1m",  // User override respected
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      1,
			},
		},
		{
			name: "User-provided CleanupDeadServers override (disabled)",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								CleanupDeadServers: ptrTo(false),
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // User override respected
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      1,
			},
		},
		{
			name: "User-provided LastContactThreshold override",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								LastContactThreshold: "30s",
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "30s", // User override respected
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      1,
			},
		},
		{
			name: "User-provided MaxTrailingLogs override",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 1,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								MaxTrailingLogs: ptrToInt32(2000),
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                2000, // User override respected
				ServerStabilizationTime:        "10s",
				MinQuorum:                      1,
			},
		},
		{
			name: "All user overrides provided",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileHardened,
					Replicas: 3,
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						Raft: &openbaov1alpha1.RaftConfig{
							Autopilot: &openbaov1alpha1.RaftAutopilotConfig{
								CleanupDeadServers:             ptrTo(false),
								DeadServerLastContactThreshold: "10m",
								LastContactThreshold:           "30s",
								MaxTrailingLogs:                ptrToInt32(2000),
								ServerStabilizationTime:        "20s",
								MinQuorum:                      ptrToInt32(7),
							},
						},
					},
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false,
				DeadServerLastContactThreshold: "10m",
				LastContactThreshold:           "30s",
				MaxTrailingLogs:                2000,
				ServerStabilizationTime:        "20s",
				MinQuorum:                      7,
			},
		},
		{
			name: "Development profile with 0 replicas (edge case, should use 1)",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Replicas: 0,
				},
			},
			want: openbao.AutopilotConfig{
				CleanupDeadServers:             false, // Auto-disabled because MinQuorum < 3
				DeadServerLastContactThreshold: "5m",
				LastContactThreshold:           "10s",
				MaxTrailingLogs:                1000,
				ServerStabilizationTime:        "10s",
				MinQuorum:                      1, // Minimum enforced
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAutopilotConfig(tt.cluster)

			if got.CleanupDeadServers != tt.want.CleanupDeadServers {
				t.Errorf("CleanupDeadServers = %v, want %v", got.CleanupDeadServers, tt.want.CleanupDeadServers)
			}
			if got.DeadServerLastContactThreshold != tt.want.DeadServerLastContactThreshold {
				t.Errorf("DeadServerLastContactThreshold = %q, want %q", got.DeadServerLastContactThreshold, tt.want.DeadServerLastContactThreshold)
			}
			if got.ServerStabilizationTime != tt.want.ServerStabilizationTime {
				t.Errorf("ServerStabilizationTime = %q, want %q", got.ServerStabilizationTime, tt.want.ServerStabilizationTime)
			}
			if got.MinQuorum != tt.want.MinQuorum {
				t.Errorf("MinQuorum = %d, want %d", got.MinQuorum, tt.want.MinQuorum)
			}
			if got.LastContactThreshold != tt.want.LastContactThreshold {
				t.Errorf("LastContactThreshold = %q, want %q", got.LastContactThreshold, tt.want.LastContactThreshold)
			}
			if got.MaxTrailingLogs != tt.want.MaxTrailingLogs {
				t.Errorf("MaxTrailingLogs = %d, want %d", got.MaxTrailingLogs, tt.want.MaxTrailingLogs)
			}
		})
	}
}
