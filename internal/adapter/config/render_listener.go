package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func buildListenerBlock(cluster *openbaov1alpha1.OpenBaoCluster) (*hclwrite.Block, error) {
	listener := hclListenerTCP{
		Type:               "tcp",
		Address:            fmt.Sprintf("[::]:%d", openBaoAPIPort),
		ClusterAddress:     fmt.Sprintf("[::]:%d", openBaoClusterPort),
		TLSDisable:         0,
		MaxRequestDuration: configMaxRequestDuration,
	}

	if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.Listener != nil {
		listener.ProxyProtocolBehavior = stringPtr(cluster.Spec.Configuration.Listener.ProxyProtocolBehavior)
		if cluster.Spec.Configuration.Listener.TLSDisable != nil {
			if *cluster.Spec.Configuration.Listener.TLSDisable {
				listener.TLSDisable = 1
			} else {
				listener.TLSDisable = 0
			}
		}
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME {
		if cluster.Spec.TLS.ACME == nil {
			return nil, fmt.Errorf("ACME configuration is required when tls.mode is ACME")
		}
		if strings.TrimSpace(cluster.Spec.TLS.ACME.Domain) != "" && len(cluster.Spec.TLS.ACME.Domains) > 0 {
			return nil, fmt.Errorf("tls.acme.domain and tls.acme.domains are mutually exclusive; use only one")
		}
		listener.TLSACMECADir = stringPtr(cluster.Spec.TLS.ACME.DirectoryURL)
		domains := portopenbao.ComputeACMEDomains(cluster)
		listener.TLSACMEDomains = &domains
		listener.TLSACMEEmail = stringPtr(cluster.Spec.TLS.ACME.Email)
		listener.TLSACMECachePath = stringPtr(portopenbao.ACMESharedCachePath(cluster))
		listener.TLSACMEDisableHTTPChallenge = boolPtrValue(true)
		if cluster.Spec.Configuration != nil {
			listener.TLSACMECARoot = stringPtr(cluster.Spec.Configuration.ACMECARoot)
		}
	} else {
		listener.TLSCertFile = stringPtr(openBaoPathTLSServerCert)
		listener.TLSKeyFile = stringPtr(openBaoPathTLSServerKey)
		listener.TLSClientCAFile = stringPtr(openBaoPathTLSCACert)
	}

	return gohcl.EncodeAsBlock(listener, "listener"), nil
}
