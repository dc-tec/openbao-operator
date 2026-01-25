/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	certmanagerVersion = "v1.19.1"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"

	defaultKindBinary  = "kind"
	defaultKindCluster = "kind"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// lookPathNoDot behaves like exec.LookPath, but ignores empty/ "." PATH entries.
//
// This avoids Go's security behavior (exec.ErrDot) where running binaries found via the
// current directory is refused when PATH contains "."/empty segments.
func lookPathNoDot(file string) (string, error) {
	if strings.ContainsRune(file, os.PathSeparator) {
		return file, nil
	}

	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" || dir == "." {
			continue
		}
		// Expand ~ to home directory if present
		if strings.HasPrefix(dir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			dir = filepath.Join(home, dir[2:])
		}
		candidate := filepath.Join(dir, file)
		st, err := os.Stat(candidate)
		if err != nil || st.IsDir() {
			continue
		}
		// Any executable bit.
		if st.Mode()&0o111 != 0 {
			// Return absolute path to avoid any relative path issues
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs, nil
			}
			return candidate, nil
		}
	}

	return "", fmt.Errorf("executable %q not found in PATH (ignoring '.'/empty entries)", file)
}

func resolveCmdPath(cmd *exec.Cmd) error {
	// If the caller already provided a path (e.g. ./tool or /abs/tool), keep it.
	if cmd.Path == "" || strings.ContainsRune(cmd.Path, os.PathSeparator) {
		return nil
	}

	p, err := lookPathNoDot(cmd.Path)
	if err == nil {
		cmd.Path = p
		return nil
	}

	// Best-effort fallback to exec.LookPath (for odd environments).
	// If LookPath finds the binary via "." in PATH, we get exec.ErrDot.
	// In that case, try to find it in PATH without "." entries by manually searching.
	lp, lpErr := exec.LookPath(cmd.Path)
	if lpErr == nil {
		// If LookPath returned a relative path, make it explicit/absolute.
		abs, absErr := filepath.Abs(lp)
		if absErr == nil {
			cmd.Path = abs
		} else {
			cmd.Path = lp
		}
		return nil
	}
	if errors.Is(lpErr, exec.ErrDot) {
		// exec.LookPath found the binary in "." but Go refuses to run it.
		// Try harder to find it in PATH without "." entries.
		// This can happen in CI if PATH contains "." and there's a file with the same name.
		if p, retryErr := lookPathNoDot(cmd.Path); retryErr == nil {
			cmd.Path = p
			return nil
		}
		return fmt.Errorf(
			"refusing to run %q found via current directory; "+
				"fix PATH (remove '.'/empty entries) or set an explicit tool path (e.g. KIND=/abs/path/to/kind): %w",
			cmd.Path, lpErr)
	}

	// Preserve original error message context.
	return err
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	if err := resolveCmdPath(cmd); err != nil {
		return "", err
	}
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	// #nosec G204 -- Test utility, command and arguments are controlled
	cmd := exec.Command("kubectl", "delete", "-f", url, "--ignore-not-found")
	output, err := Run(cmd)
	// kubectl delete with --ignore-not-found can still return errors, but if resources were deleted
	// (indicated by "deleted" in output), we consider it successful
	if err != nil && !strings.Contains(output, "deleted") {
		warnError(err)
	}

	// Delete leftover leases in kube-system (not cleaned by default)
	kubeSystemLeases := []string{
		"cert-manager-cainjector-leader-election",
		"cert-manager-controller",
	}
	for _, lease := range kubeSystemLeases {
		// #nosec G204 -- Test utility, command and arguments are controlled
		cmd = exec.Command("kubectl", "delete", "lease", lease,
			"-n", "kube-system", "--ignore-not-found", "--force", "--grace-period=0")
		if _, err := Run(cmd); err != nil {
			warnError(err)
		}
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url) // #nosec G204 -- Test utility, command and arguments are controlled
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

const (
	csiHostPathVersion = "v1.9.0"
	// Note: we intentionally apply upstream manifests from a pinned tag.
	// The hostpath CSI driver is a test driver; we use it only for E2E fidelity.
	csiHostPathPluginURLTmpl = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/%s/" +
		"deploy/kubernetes-1.21/hostpath/csi-hostpath-plugin.yaml"
	csiHostPathDriverInfoURLTmpl = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/csi-driver-host-path/%s/" +
		"deploy/kubernetes-1.21/hostpath/csi-hostpath-driverinfo.yaml"

	csiHostPathProvisionerRBACURL = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/v3.2.0/" +
		"deploy/kubernetes/rbac.yaml"
	csiHostPathAttacherRBACURL = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/external-attacher/v3.5.0/" +
		"deploy/kubernetes/rbac.yaml"
	csiHostPathResizerRBACURL = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/external-resizer/v1.5.0/" +
		"deploy/kubernetes/rbac.yaml"
	csiHostPathSnapshotterRBACURL = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v6.0.1/" +
		"deploy/kubernetes/csi-snapshotter/rbac-csi-snapshotter.yaml"
	csiHostPathHealthMonitorRBACURL = "" +
		"https://raw.githubusercontent.com/kubernetes-csi/external-health-monitor/v0.5.0/" +
		"deploy/kubernetes/external-health-monitor-controller/rbac.yaml"
)

// InstallCSIHostPathDriver installs the CSI hostpath driver (test driver) and waits for it to become ready.
// This enables full-path PVC expansion testing in Kind.
func InstallCSIHostPathDriver() error {
	pluginURL := fmt.Sprintf(csiHostPathPluginURLTmpl, csiHostPathVersion)
	driverInfoURL := fmt.Sprintf(csiHostPathDriverInfoURLTmpl, csiHostPathVersion)

	// The upstream hostpath plugin manifest references RBAC roles that are shipped with CSI sidecars.
	// Apply those RBAC manifests first so provisioning can work (cluster roles + namespaced roles).
	for _, url := range []string{
		csiHostPathProvisionerRBACURL,
		csiHostPathAttacherRBACURL,
		csiHostPathResizerRBACURL,
		csiHostPathSnapshotterRBACURL,
		csiHostPathHealthMonitorRBACURL,
	} {
		cmd := exec.Command("kubectl", "apply", "-n", "default", "-f", url) // #nosec G204 -- test harness
		if _, err := Run(cmd); err != nil {
			return err
		}
	}

	cmd := exec.Command("kubectl", "apply", "-f", pluginURL) // #nosec G204 -- test harness
	if _, err := Run(cmd); err != nil {
		return err
	}

	cmd = exec.Command("kubectl", "apply", "-f", driverInfoURL) // #nosec G204 -- test harness
	if _, err := Run(cmd); err != nil {
		return err
	}

	// The upstream manifest deploys a single-replica StatefulSet in the "default" namespace.
	cmd = exec.Command(
		"kubectl", "-n", "default", "rollout", "status",
		"statefulset/csi-hostpathplugin", "--timeout=5m",
	) // #nosec G204 -- test harness
	_, err := Run(cmd)
	return err
}

// UninstallCSIHostPathDriver removes the CSI hostpath driver resources (best effort).
func UninstallCSIHostPathDriver() {
	pluginURL := fmt.Sprintf(csiHostPathPluginURLTmpl, csiHostPathVersion)
	driverInfoURL := fmt.Sprintf(csiHostPathDriverInfoURLTmpl, csiHostPathVersion)

	cmd := exec.Command("kubectl", "delete", "-f", driverInfoURL, "--ignore-not-found") // #nosec G204 -- test harness
	_, _ = Run(cmd)
	cmd = exec.Command("kubectl", "delete", "-f", pluginURL, "--ignore-not-found") // #nosec G204 -- test harness
	_, _ = Run(cmd)
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := defaultKindCluster
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	kindBinary := defaultKindBinary
	if v, ok := os.LookupEnv("KIND"); ok {
		kindBinary = v
	}
	cmd := exec.Command(kindBinary, kindOptions...) // #nosec G204 -- Test utility, command and arguments are controlled
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	content, err := os.ReadFile(filename) // #nosec G304 -- Test utility, filename is controlled
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// #nosec G306 -- File is intentionally readable by group/others (test utility)
	if err = os.WriteFile(filename, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
