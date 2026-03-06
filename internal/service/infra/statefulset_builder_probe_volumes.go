package infra

import (
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func buildStatefulSetProbeExecActions(cluster *openbaov1alpha1.OpenBaoCluster) probeExecActions {
	// Probe target/CA: by default use loopback and the per-cluster TLS CA.
	probeAddr := openBaoProbeAddr
	probeCAFile := openBaoProbeCAFile
	var probeServerName string
	if usesACMEMode(cluster) && cluster.Spec.TLS.ACME != nil {
		// In ACME mode, keep probes on loopback but set SNI to the ACME domain.
		// This prevents OpenBao from attempting ACME for "localhost" while avoiding
		// DNS/service dependencies for probes.
		domains := acmeDomains(cluster)
		if len(domains) > 0 {
			probeServerName = domains[0]
		}
		// In ACME mode, probes need to verify the ACME-obtained certificate, which is signed
		// by the ACME CA (PKI CA), not the ACME directory server's TLS CA.
		// If tls_acme_ca_root is configured, derive the PKI CA path from it (same directory,
		// filename pki-ca.crt). This allows users to provide the PKI CA in the same volume.
		// If not available, use system roots (for public ACME CAs like Let's Encrypt).
		if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.ACMECARoot != "" {
			// Derive PKI CA path from tls_acme_ca_root: same directory, filename pki-ca.crt
			// e.g., /etc/bao/seal-creds/ca.crt -> /etc/bao/seal-creds/pki-ca.crt
			acmeCARootDir := path.Dir(cluster.Spec.Configuration.ACMECARoot)
			probeCAFile = path.Join(acmeCARootDir, "pki-ca.crt")
		} else {
			// No tls_acme_ca_root configured - use system roots for public ACME CAs
			probeCAFile = ""
		}
	}

	// For non-ACME TLS modes, the server certificate often contains DNS SANs (Service/Gateway/Ingress)
	// but not the loopback IP. Probes connect via loopback for locality, so we must provide an SNI
	// server name that matches a DNS SAN to avoid x509 "no IP SANs" failures.
	//
	// Only set this when an external-facing Service is expected to exist; otherwise, the
	// operator-managed default certificate includes 127.0.0.1 and probes can rely on IP validation.
	if probeServerName == "" && cluster.Spec.TLS.Enabled && !usesACMEMode(cluster) {
		if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
			if hn := strings.TrimSpace(cluster.Spec.Gateway.Hostname); hn != "" {
				probeServerName = hn
			}
		}
		if probeServerName == "" && cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
			if host := strings.TrimSpace(cluster.Spec.Ingress.Host); host != "" {
				probeServerName = host
			}
		}

		needsExternalService := cluster.Spec.Service != nil ||
			(cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled) ||
			(cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled)
		if probeServerName == "" && needsExternalService {
			probeServerName = fmt.Sprintf("%s.%s.svc", externalServiceName(cluster), cluster.Namespace)
		}
	}

	// Startup probe only does TCP dial, so it doesn't need a CA file.
	// In ACME mode, the default CA file (/etc/bao/tls/ca.crt) doesn't exist, so
	// explicitly set -ca-file="" to avoid trying to read it.
	startupProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=startup",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoStartupProbeTimeout,
	}
	// Only set empty CA file for ACME mode where the default CA file won't exist
	if usesACMEMode(cluster) {
		startupProbeCmd = append(startupProbeCmd, "-ca-file=")
	}
	startupProbeExec := &corev1.ExecAction{
		Command: startupProbeCmd,
	}

	livenessProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=liveness",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoLivenessProbeTimeout,
	}
	if probeServerName != "" {
		livenessProbeCmd = append(livenessProbeCmd, "-servername="+probeServerName)
	}
	if probeCAFile != "" {
		livenessProbeCmd = append(livenessProbeCmd, "-ca-file="+probeCAFile)
	}
	livenessProbeExec := &corev1.ExecAction{
		Command: livenessProbeCmd,
	}

	readinessProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=readiness",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoReadinessProbeTimeout,
	}
	if probeServerName != "" {
		readinessProbeCmd = append(readinessProbeCmd, "-servername="+probeServerName)
	}
	if probeCAFile != "" {
		readinessProbeCmd = append(readinessProbeCmd, "-ca-file="+probeCAFile)
	}
	readinessProbeExec := &corev1.ExecAction{
		Command: readinessProbeCmd,
	}

	return probeExecActions{
		startup:   startupProbeExec,
		liveness:  livenessProbeExec,
		readiness: readinessProbeExec,
	}
}

func acmeDomains(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	if cluster == nil || cluster.Spec.TLS.ACME == nil {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(cluster.Spec.TLS.ACME.Domains)+1)
	for _, raw := range cluster.Spec.TLS.ACME.Domains {
		d := strings.TrimSpace(raw)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}

	if len(out) == 0 {
		if d := strings.TrimSpace(cluster.Spec.TLS.ACME.Domain); d != "" {
			out = append(out, d)
		}
	}

	if len(out) == 0 {
		out = append(out, fmt.Sprintf("%s-acme.%s.svc", cluster.Name, cluster.Namespace))
	}

	return out
}

func buildStatefulSetVolumes(cluster *openbaov1alpha1.OpenBaoCluster, revision string, disableSelfInit bool) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapNameWithRevision(cluster, revision),
					},
				},
			},
		},
		{
			Name: configRenderedVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: tmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: utilsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: kubeAPIAccessVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					DefaultMode: ptr.To(serviceAccountFileMode),
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              "token",
								ExpirationSeconds: ptr.To(serviceAccountTokenExpirationSeconds),
							},
						},
						{
							ConfigMap: &corev1.ConfigMapProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: kubeRootCAConfigMapName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  "ca.crt",
										Path: "ca.crt",
									},
								},
							},
						},
						{
							DownwardAPI: &corev1.DownwardAPIProjection{
								Items: []corev1.DownwardAPIVolumeFile{
									{
										Path: "namespace",
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Only add TLS volume when not using ACME mode (ACME stores certs in /bao/data)
	if !usesACMEMode(cluster) {
		volumes = append(volumes, corev1.Volume{
			Name: tlsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsServerSecretName(cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		})
	}

	volumes = append(volumes, newSealWiringProvider(cluster).Volumes()...)

	// If self-init is enabled, add the self-init ConfigMap volume, unless disabled (Green pods)
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled && !disableSelfInit {
		volumes = append(volumes, corev1.Volume{
			Name: configInitVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configInitMapName(cluster),
					},
				},
			},
		})
	}

	return volumes
}

// buildStatefulSet constructs a StatefulSet for the given OpenBaoCluster.
// This is a convenience wrapper that calls buildStatefulSetWithRevision with an empty revision.
//

// buildStatefulSetWithRevision constructs a StatefulSet for the given OpenBaoCluster.
// verifiedImageDigest is the verified image digest to use (if provided, overrides cluster.Spec.Image).
// verifiedInitContainerDigest is the verified init container image digest to use (if provided, overrides cluster.Spec.InitContainer.Image).
// revision is an optional revision identifier for blue/green deployments.
// disableSelfInit prevents adding self-init logic (used for Green pods).
