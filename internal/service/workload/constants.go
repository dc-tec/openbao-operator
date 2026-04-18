package workload

import "github.com/dc-tec/openbao-operator/internal/platform/constants"

const (
	configInitMapSuffix      = "-config-init"
	dataVolumeName           = constants.VolumeData
	tlsVolumeName            = constants.VolumeTLS
	configVolumeName         = constants.VolumeConfig
	configInitVolumeName     = "config-init"
	configRenderedVolumeName = "config-rendered"
	unsealVolumeName         = "unseal"
	tmpVolumeName            = "tmp"
	utilsVolumeName          = "utils"
	acmeCacheVolumeName      = "acme-cache"
	kubeAPIAccessVolumeName  = "kube-api-access"
	configFileName           = "config.hcl"
	configTemplatePath       = "/etc/bao/config/config.hcl"
	configInitTemplatePath   = "/etc/bao/config-init/config.hcl"
	openBaoConfigMountPath   = constants.PathConfig
	openBaoRenderedConfig    = "/etc/bao/rendered-config/config.hcl"
	openBaoTLSMountPath      = constants.PathTLS
	openBaoUnsealMountPath   = "/etc/bao/unseal"
	openBaoDataPath          = constants.PathData
	serviceAccountMountPath  = "/var/run/secrets/kubernetes.io/serviceaccount"
	kubeRootCAConfigMapName  = "kube-root-ca.crt"
	openBaoBinaryName        = constants.BinaryBao
	configHashAnnotation     = "openbao.org/config-hash"
	unsealTypeTransit        = "transit"

	openBaoUserID  = constants.UserOpenBao
	openBaoGroupID = constants.GroupOpenBao

	secretFileMode                       = int32(0440)
	serviceAccountFileMode               = int32(0440)
	serviceAccountTokenExpirationSeconds = int64(3600)
)
