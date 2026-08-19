package workload

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestStatefulSet_OCIPluginAutoDownloadMountsWritablePluginDirectory(t *testing.T) {
	cluster := newMinimalCluster("oci-plugins", "default")
	cluster.Spec.Version = "2.6.2"
	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
		Plugin: &openbaov1alpha1.PluginConfig{
			AutoDownload: ptr.To(true),
		},
	}
	cluster.Spec.Plugins = []openbaov1alpha1.Plugin{
		{
			Type:       "secret",
			Name:       "nats",
			Image:      "ghcr.io/example/openbao-plugin",
			Version:    "1.2.3",
			BinaryName: "openbao-plugin",
			SHA256Sum:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	pluginVolume, ok := getVolume(statefulSet.Spec.Template.Spec.Volumes, pluginVolumeName)
	if !ok {
		t.Fatalf("expected %q volume to be present", pluginVolumeName)
	}
	if pluginVolume.EmptyDir == nil {
		t.Fatalf("expected %q volume to use emptyDir", pluginVolumeName)
	}

	pluginMount, ok := getVolumeMount(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, pluginVolumeName)
	if !ok {
		t.Fatalf("expected OpenBao container to mount %q volume", pluginVolumeName)
	}
	if pluginMount.MountPath != openBaoPluginPath {
		t.Fatalf("plugin mount path = %q, want %q", pluginMount.MountPath, openBaoPluginPath)
	}
	if pluginMount.ReadOnly {
		t.Fatalf("expected %q mount to be writable", pluginVolumeName)
	}
}

func TestStatefulSet_OCIPluginDirectoryVolumeOmittedWhenAutoDownloadUnused(t *testing.T) {
	tests := []struct {
		name         string
		autoDownload *bool
		plugins      []openbaov1alpha1.Plugin
	}{
		{
			name:         "no plugin config",
			autoDownload: nil,
			plugins: []openbaov1alpha1.Plugin{
				{Type: "secret", Name: "nats", Image: "ghcr.io/example/openbao-plugin"},
			},
		},
		{
			name:         "auto download disabled",
			autoDownload: ptr.To(false),
			plugins: []openbaov1alpha1.Plugin{
				{Type: "secret", Name: "nats", Image: "ghcr.io/example/openbao-plugin"},
			},
		},
		{
			name:         "auto download enabled without plugins",
			autoDownload: ptr.To(true),
		},
		{
			name:         "auto download enabled for command plugin",
			autoDownload: ptr.To(true),
			plugins: []openbaov1alpha1.Plugin{
				{Type: "secret", Name: "local", Command: "/usr/local/bin/openbao-plugin"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalCluster("oci-plugins-unused", "default")
			cluster.Spec.Version = "2.6.2"
			if tt.autoDownload != nil {
				cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
					Plugin: &openbaov1alpha1.PluginConfig{
						AutoDownload: tt.autoDownload,
					},
				}
			}
			cluster.Spec.Plugins = tt.plugins

			statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
			if err != nil {
				t.Fatalf("buildStatefulSet() error = %v", err)
			}

			if hasVolume(statefulSet.Spec.Template.Spec.Volumes, pluginVolumeName) {
				t.Fatalf("did not expect %q volume", pluginVolumeName)
			}
			if hasVolumeMount(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, pluginVolumeName) {
				t.Fatalf("did not expect OpenBao container to mount %q volume", pluginVolumeName)
			}
		})
	}
}

func getVolumeMount(mounts []corev1.VolumeMount, name string) (*corev1.VolumeMount, bool) {
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i], true
		}
	}
	return nil, false
}
