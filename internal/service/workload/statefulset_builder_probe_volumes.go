package workload

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func buildStatefulSetProbeExecActions(cluster *openbaov1alpha1.OpenBaoCluster) probeExecActions {
	// Probe target/CA: by default use loopback and the per-cluster TLS CA.
	probeAddr := openBaoProbeAddr
	probeCAFile := openBaoProbeCAFile
	probeServerName := portopenbao.ComputeTLSServerName(cluster)
	if usesACMEMode(cluster) && cluster.Spec.TLS.ACME != nil {
		// In ACME mode, keep probes on loopback but verify against the effective ACME
		// TLS name rather than the connection hostname. This avoids triggering ACME for
		// localhost while keeping certificate validation aligned with day-2 clients.
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

	startupProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=startup",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoStartupProbeTimeout,
	}
	if probeServerName != "" {
		startupProbeCmd = append(startupProbeCmd, "-servername="+probeServerName)
	}
	if probeCAFile != "" {
		startupProbeCmd = append(startupProbeCmd, "-ca-file="+probeCAFile)
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

func buildStatefulSetVolumes(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapNameForSpec(cluster, spec),
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

	// Only add TLS volume when not using ACME mode (ACME stores certs in OpenBao's
	// internal ACME cache rather than in a mounted Kubernetes TLS Secret).
	if !usesACMEMode(cluster) {
		volumes = append(volumes, corev1.Volume{
			Name: tlsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  resourceidentity.TLSServerSecretName(cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		})
	}

	if claimName := portopenbao.ACMESharedCacheClaimName(cluster); claimName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: acmeCacheVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	if claimName := portopenbao.AuditFileStorageClaimName(cluster); claimName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: auditFileStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	if usesDeclarativeOCIPluginDownload(cluster) {
		volumes = append(volumes, corev1.Volume{
			Name: pluginVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	volumes = append(volumes, newSealWiringProvider(cluster).Volumes()...)

	// If self-init is enabled, add the self-init ConfigMap volume, unless disabled (Green pods)
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled && !spec.DisableSelfInit {
		volumes = append(volumes, corev1.Volume{
			Name: configInitVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: resourceidentity.ConfigInitMapName(cluster),
					},
				},
			},
		})
	}

	return volumes
}
