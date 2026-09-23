package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	platformsemver "github.com/dc-tec/openbao-operator/internal/platform/semver"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func validatePluginVersionCompatibility(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if len(cluster.Spec.Plugins) == 0 {
		return nil
	}
	version, err := platformsemver.Parse(cluster.Spec.Version)
	if err != nil {
		return fmt.Errorf("validate plugin compatibility: %w", err)
	}
	modern := version.Major() > 2 || (version.Major() == 2 && version.Minor() >= 7)
	for _, plugin := range cluster.Spec.Plugins {
		if err := validatePluginDeclaration(plugin, modern); err != nil {
			return fmt.Errorf("spec.plugins[%s/%s]: %w", plugin.Type, plugin.Name, err)
		}
	}
	return nil
}

func validatePluginDeclaration(plugin openbaov1alpha1.Plugin, modern bool) error {
	if (plugin.Image == "") == (plugin.Command == "") {
		return fmt.Errorf("exactly one of image or command is required")
	}
	if !modern && (plugin.Version == "" || plugin.BinaryName == "" || plugin.SHA256Sum == "") {
		return fmt.Errorf("version, binaryName, and sha256sum are required before OpenBao 2.7.0")
	}
	if plugin.Command != "" {
		command := path.Clean(plugin.Command)
		if path.IsAbs(command) || command == "." || command == ".." || strings.HasPrefix(command, "../") {
			return fmt.Errorf("command must be relative to the plugin directory")
		}
		if plugin.Version == "" && plugin.Type != portopenbao.SealTypeKMSPlugin {
			return fmt.Errorf("version is required for command-based auth, secret, and database plugins")
		}
		return nil
	}
	ref, err := name.ParseReference(plugin.Image)
	if err != nil {
		return fmt.Errorf("invalid image reference: %w", err)
	}
	if _, pinned := ref.(name.Digest); !pinned && plugin.SHA256Sum == "" {
		return fmt.Errorf("sha256sum is required unless image is pinned by digest")
	}
	tag, err := name.NewTag(strings.SplitN(plugin.Image, "@", 2)[0], name.WithDefaultTag(plugin.Version))
	if err != nil {
		return fmt.Errorf("invalid image tag: %w", err)
	}
	if tag.Identifier() == "" {
		return fmt.Errorf("version is required when image has no tag")
	}
	return nil
}
