package openbao

import (
	"fmt"
	"path"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	sealCredsMountPath     = "/etc/bao/seal-creds"
	privateACMEPKICASecret = "pki-ca.crt"
)

// TrustBundleSource describes how helper binaries and controller-side clients
// should trust the cluster API certificate.
type TrustBundleSource struct {
	SecretName     string
	SecretKey      string
	UseSystemRoots bool
}

// ComputeTLSServerName returns the stable name that internal clients should use
// for TLS verification when connecting to OpenBao Pods directly.
func ComputeTLSServerName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || !cluster.Spec.TLS.Enabled || strings.TrimSpace(cluster.Name) == "" {
		return ""
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode == openbaov1alpha1.TLSModeACME && cluster.Spec.TLS.ACME != nil {
		if d := preferredACMETLSServerName(ComputeACMEDomains(cluster)); d != "" {
			return d
		}
	}

	return fmt.Sprintf("openbao-cluster-%s.local", cluster.Name)
}

// ComputeACMEDomains returns the effective ACME domains in operator order.
func ComputeACMEDomains(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	if cluster == nil || cluster.Spec.TLS.ACME == nil {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(cluster.Spec.TLS.ACME.Domains))
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

	if len(out) == 0 && cluster != nil {
		out = append(out, fmt.Sprintf("%s-acme.%s.svc", cluster.Name, cluster.Namespace))
	}

	return out
}

// ResolveClientTrustBundle determines which trust source internal clients should
// use for the cluster API.
func ResolveClientTrustBundle(cluster *openbaov1alpha1.OpenBaoCluster) (TrustBundleSource, error) {
	if cluster == nil {
		return TrustBundleSource{}, fmt.Errorf("cluster is required")
	}
	if !cluster.Spec.TLS.Enabled {
		return TrustBundleSource{UseSystemRoots: true}, nil
	}

	tlsMode := cluster.Spec.TLS.Mode
	if tlsMode == "" {
		tlsMode = openbaov1alpha1.TLSModeOperatorManaged
	}

	if tlsMode != openbaov1alpha1.TLSModeACME {
		return TrustBundleSource{
			SecretName: cluster.Name + constants.SuffixTLSCA,
			SecretKey:  "ca.crt",
		}, nil
	}

	if cluster.Spec.Configuration == nil || strings.TrimSpace(cluster.Spec.Configuration.ACMECARoot) == "" {
		return TrustBundleSource{UseSystemRoots: true}, nil
	}

	acmeCARoot := path.Clean(strings.TrimSpace(cluster.Spec.Configuration.ACMECARoot))
	cleanMount := path.Clean(sealCredsMountPath)
	if acmeCARoot == "." || acmeCARoot == "" || acmeCARoot == cleanMount || !strings.HasPrefix(acmeCARoot, cleanMount+"/") {
		return TrustBundleSource{}, fmt.Errorf(
			"private ACME trust requires spec.configuration.acmeCARoot to reference a file under %s",
			sealCredsMountPath,
		)
	}

	if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.CredentialsSecretRef == nil || strings.TrimSpace(cluster.Spec.Unseal.CredentialsSecretRef.Name) == "" {
		return TrustBundleSource{}, fmt.Errorf(
			"private ACME trust requires spec.unseal.credentialsSecretRef so the operator can mount %q for probes and day-2 operations",
			privateACMEPKICASecret,
		)
	}

	return TrustBundleSource{
		SecretName: cluster.Spec.Unseal.CredentialsSecretRef.Name,
		SecretKey:  privateACMEPKICASecret,
	}, nil
}

func preferredACMETLSServerName(domains []string) string {
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if strings.Contains(d, ".svc") {
			return d
		}
	}
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d != "" {
			return d
		}
	}
	return ""
}
