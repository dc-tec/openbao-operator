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

package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	certmanager "github.com/openbao/operator/internal/certs"
	openbaoclustercontroller "github.com/openbao/operator/internal/controller/openbaocluster"
	initmanager "github.com/openbao/operator/internal/init"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))
}

// discoverOIDC fetches the Kubernetes OIDC issuer configuration at operator startup.
// TIGHTENED: Fail fast if unreachable. Do not fallback to guessing.
// baseURL allows tests (or specialized environments) to override the default
// Kubernetes API DNS name. When empty, it defaults to:
//
//	https://kubernetes.default.svc
//
// Returns empty strings if discovery fails (operator can still run for Development profile clusters).
func discoverOIDC(
	ctx context.Context,
	cfg *rest.Config,
	baseURL string,
) (issuerURL string, caBundle string, err error) {
	// 1. Try well-known endpoint using K8s client transport
	if baseURL == "" {
		baseURL = "https://kubernetes.default.svc"
	}
	wellKnownURL := baseURL + "/.well-known/openid-configuration"

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create OIDC discovery request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// TIGHTENED: Return error instead of fallback
		return "", "", fmt.Errorf("failed to fetch OIDC well-known endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("OIDC well-known endpoint returned status %d", resp.StatusCode)
	}

	var oidcConfig struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return "", "", fmt.Errorf("failed to parse OIDC config: %w", err)
	}

	if oidcConfig.Issuer == "" {
		return "", "", fmt.Errorf("OIDC config missing issuer")
	}

	issuerURL = oidcConfig.Issuer

	// 2. Get CA bundle from REST config
	if len(cfg.CAData) > 0 {
		caBundle = string(cfg.CAData)
	} else if cfg.CAFile != "" {
		data, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read CA file: %w", err)
		}
		caBundle = string(data)
	} else {
		// No CA configured - use system cert pool (may be empty)
		caBundle = ""
	}

	return issuerURL, caBundle, nil
}

type oidcDiscoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`

	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`

	N string `json:"n,omitempty"`
	E string `json:"e,omitempty"`

	X5c []string `json:"x5c,omitempty"`
}

func pemPublicKeysFromJWKS(jwks jwksDocument) ([]string, error) {
	var pemKeys []string
	seen := make(map[string]struct{}, len(jwks.Keys))

	for _, key := range jwks.Keys {
		if len(key.X5c) > 0 {
			certDER, err := base64.StdEncoding.DecodeString(key.X5c[0])
			if err != nil {
				return nil, fmt.Errorf("failed to decode jwk x5c certificate: %w", err)
			}

			cert, err := x509.ParseCertificate(certDER)
			if err != nil {
				return nil, fmt.Errorf("failed to parse jwk x5c certificate: %w", err)
			}

			pubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal jwk x5c public key: %w", err)
			}

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
			if _, ok := seen[pemKey]; ok {
				continue
			}
			seen[pemKey] = struct{}{}
			pemKeys = append(pemKeys, pemKey)
			continue
		}

		switch key.Kty {
		case "RSA":
			nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
			if err != nil {
				return nil, fmt.Errorf("failed to decode rsa modulus: %w", err)
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
			if err != nil {
				return nil, fmt.Errorf("failed to decode rsa exponent: %w", err)
			}
			if len(eBytes) == 0 {
				return nil, fmt.Errorf("rsa exponent is empty")
			}

			exponent := 0
			for _, b := range eBytes {
				exponent = exponent<<8 | int(b)
			}

			pubKey := &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: exponent,
			}

			pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal rsa public key: %w", err)
			}

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
			if _, ok := seen[pemKey]; ok {
				continue
			}
			seen[pemKey] = struct{}{}
			pemKeys = append(pemKeys, pemKey)
		case "EC":
			var curve elliptic.Curve
			switch key.Crv {
			case "P-256":
				curve = elliptic.P256()
			case "P-384":
				curve = elliptic.P384()
			case "P-521":
				curve = elliptic.P521()
			default:
				return nil, fmt.Errorf("unsupported ec curve %q", key.Crv)
			}

			xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ec x coordinate: %w", err)
			}
			yBytes, err := base64.RawURLEncoding.DecodeString(key.Y)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ec y coordinate: %w", err)
			}

			pubKey := &ecdsa.PublicKey{
				Curve: curve,
				X:     new(big.Int).SetBytes(xBytes),
				Y:     new(big.Int).SetBytes(yBytes),
			}

			pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal ec public key: %w", err)
			}

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
			if _, ok := seen[pemKey]; ok {
				continue
			}
			seen[pemKey] = struct{}{}
			pemKeys = append(pemKeys, pemKey)
		default:
			return nil, fmt.Errorf("unsupported jwk key type %q", key.Kty)
		}
	}

	if len(pemKeys) == 0 {
		return nil, fmt.Errorf("no public keys found in jwks")
	}

	return pemKeys, nil
}

func fetchOIDCJWKSKeys(ctx context.Context, cfg *rest.Config, jwksURL string) ([]string, error) {
	if jwksURL == "" {
		return nil, fmt.Errorf("jwks URL is required")
	}

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create jwks request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jwks endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks endpoint returned status %d", resp.StatusCode)
	}

	var jwks jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to parse jwks document: %w", err)
	}

	keys, err := pemPublicKeysFromJWKS(jwks)
	if err != nil {
		return nil, fmt.Errorf("failed to extract public keys from jwks: %w", err)
	}

	return keys, nil
}

// Run starts the OpenBaoCluster controller manager.
// The Controller is responsible for reconciling OpenBaoCluster resources,
// managing StatefulSets, and executing upgrades.
func Run() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics server")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'.
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	// SECURITY: Disable cache for resources that the controller watches but only has
	// namespace-scoped permissions for. The controller uses namespace-scoped permissions
	// via tenant Roles, so it does not have cluster-wide list/watch permissions required
	// for cache sync. Disabling cache prevents errors during manager startup and ensures
	// the controller uses direct API calls (GET) for these resources instead of requiring
	// cluster-wide list/watch permissions.
	//
	// Resources disabled:
	// - Secrets: Prevents secret enumeration attacks (only 'get' permission granted)
	// - Jobs: Controller manages Jobs but only has namespace-scoped permissions
	// - StatefulSets: Controller manages StatefulSets but only has namespace-scoped permissions
	// - Services: Controller manages Services but only has namespace-scoped permissions
	// - ConfigMaps: Controller manages ConfigMaps but only has namespace-scoped permissions
	// - Ingress: Controller manages Ingress but only has namespace-scoped permissions
	// - NetworkPolicy: Controller manages NetworkPolicy but only has namespace-scoped permissions
	// - Roles/RoleBindings: Controller manages Roles/RoleBindings for OpenBao pod discovery but only
	//   has namespace-scoped permissions via tenant Roles
	// - ServiceAccounts: Controller manages ServiceAccounts for OpenBao clusters but only has
	//   namespace-scoped permissions via tenant Roles
	// - Pods: Controller reads Pods for health checks and leader detection but only has
	//   namespace-scoped permissions via tenant Roles
	// - PersistentVolumeClaims: Controller manages PVCs via StatefulSet volume claim templates but
	//   only has namespace-scoped permissions via tenant Roles
	// - Endpoints/EndpointSlices: Controller reads Endpoints/EndpointSlices for service discovery
	//   but only has namespace-scoped permissions via tenant Roles
	// - Gateway HTTPRoute/TLSRoute/BackendTLSPolicy: Controller manages Gateway routes but only
	//   has namespace-scoped permissions via tenant Roles. Using the uncached client avoids
	//   requiring cluster-wide list/watch on Gateway API resources.
	disableForCache := []client.Object{
		&corev1.Secret{},
		&batchv1.Job{},
		&appsv1.StatefulSet{},
		&corev1.Service{},
		&corev1.ConfigMap{},
		// The controller must not list/watch namespaces. It only operates within
		// namespaces where tenant Roles grant scoped permissions.
		&corev1.Namespace{},
		&networkingv1.Ingress{},
		&networkingv1.NetworkPolicy{},
		&rbacv1.Role{},
		&rbacv1.RoleBinding{},
		&corev1.ServiceAccount{},
		&corev1.Pod{},
		&corev1.PersistentVolumeClaim{},
		&discoveryv1.EndpointSlice{},
		&gatewayv1.HTTPRoute{},
		&gatewayv1alpha2.TLSRoute{},
		&gatewayv1.BackendTLSPolicy{},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "openbao-controller-leader.openbao.org",
		// Disable cache for resources that the controller watches but only has namespace-scoped
		// permissions for. This prevents secret enumeration attacks and eliminates cache sync
		// errors when cluster-wide permissions aren't available. The operator will make direct
		// API calls (GET) for these resources instead of using the cached client, which requires
		// cluster-wide list/watch permissions.
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: disableForCache,
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create Kubernetes clientset for ReloadSignaler
	config := mgr.GetConfig()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create Kubernetes clientset")
		os.Exit(1)
	}

	// Create TLS reload signaler that annotates pods with the active TLS
	// certificate hash. A sidecar running inside the pod can watch this
	// annotation or the mounted TLS volume and send SIGHUP locally, avoiding
	// the need for pods/exec privileges in the operator.
	reloadSignaler := certmanager.NewKubernetesReloadSignaler(clientset)

	// Create initialization manager
	initMgr := initmanager.NewManager(config, clientset)

	// Get operator namespace from POD_NAMESPACE environment variable (set by Kubernetes)
	// Default to "openbao-operator-system" for backward compatibility
	operatorNamespace := os.Getenv("POD_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = "openbao-operator-system"
		setupLog.Info("POD_NAMESPACE not set, using default", "namespace", operatorNamespace)
	} else {
		setupLog.Info("Using operator namespace from POD_NAMESPACE", "namespace", operatorNamespace)
	}

	// Discover OIDC configuration immediately at startup
	config = mgr.GetConfig()
	issuer, _, err := discoverOIDC(context.Background(), config, "")
	if err != nil {
		setupLog.Error(err, "Failed to discover Kubernetes OIDC configuration. Hardened profile requires OIDC.")
		// TIGHTENED: Do not exit; just log. If a user tries to use Hardened mode later,
		// the Reconciler will fail then. This allows the operator to run on clusters
		// without OIDC if they only use Development mode.
		issuer = ""
	} else {
		setupLog.Info("Discovered Kubernetes OIDC configuration", "issuer", issuer)
	}

	// Fetch JWKS keys for configuring OpenBao JWT auth without relying on unauthenticated
	// OIDC discovery from inside the OpenBao Pods.
	var oidcJWKSKeys []string
	if issuer != "" {
		oidcWellKnownURL := "https://kubernetes.default.svc/.well-known/openid-configuration"
		transport, err := rest.TransportFor(config)
		if err != nil {
			setupLog.Error(err, "Failed to create transport for OIDC discovery")
		} else {
			httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, oidcWellKnownURL, nil)
			if err != nil {
				setupLog.Error(err, "Failed to create OIDC discovery request")
			} else {
				resp, err := httpClient.Do(req)
				if err != nil {
					setupLog.Error(err, "Failed to fetch OIDC discovery document for jwks_uri")
				} else {
					func() {
						defer func() { _ = resp.Body.Close() }()
						if resp.StatusCode != http.StatusOK {
							statusErr := fmt.Errorf("OIDC well-known endpoint returned status %d", resp.StatusCode)
							setupLog.Error(statusErr, "Failed to fetch OIDC discovery document for jwks_uri")
							return
						}

						var doc oidcDiscoveryDocument
						if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
							setupLog.Error(err, "Failed to parse OIDC discovery document for jwks_uri")
							return
						}
						if doc.JWKSURI == "" {
							missingJWKSURI := errors.New("OIDC config missing jwks_uri")
							setupLog.Error(missingJWKSURI, "Failed to fetch OIDC discovery document for jwks_uri")
							return
						}

						keys, err := fetchOIDCJWKSKeys(context.Background(), config, doc.JWKSURI)
						if err != nil {
							setupLog.Error(err, "Failed to fetch JWKS keys; Hardened clusters require JWKS to configure operator auth")
							return
						}
						oidcJWKSKeys = keys
						setupLog.Info("Fetched OIDC JWKS public keys", "count", len(keys))
					}()
				}
			}
		}
	}

	// Pass these values into the Reconciler struct
	if err := (&openbaoclustercontroller.OpenBaoClusterReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		TLSReload:         reloadSignaler,
		InitManager:       initMgr,
		OperatorNamespace: operatorNamespace,
		OIDCIssuer:        issuer,
		OIDCJWTKeys:       oidcJWKSKeys,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenBaoCluster")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
