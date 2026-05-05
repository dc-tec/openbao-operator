package openbaoclusterclaim

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func assertProjectedLocalCluster(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	if cluster.Labels[constants.LabelOpenBaoOwnershipMode] != constants.LabelValueOpenBaoOwnershipClaimManaged {
		t.Fatalf("ownership mode label = %q, want %q", cluster.Labels[constants.LabelOpenBaoOwnershipMode], constants.LabelValueOpenBaoOwnershipClaimManaged)
	}
	if cluster.Labels[constants.LabelOpenBaoClaimNamespace] != claim.Namespace {
		t.Fatalf("claim namespace label = %q, want %q", cluster.Labels[constants.LabelOpenBaoClaimNamespace], claim.Namespace)
	}
	if cluster.Labels[constants.LabelOpenBaoClaimName] != claim.Name {
		t.Fatalf("claim name label = %q, want %q", cluster.Labels[constants.LabelOpenBaoClaimName], claim.Name)
	}
	if cluster.Spec.Version != "2.6.0" {
		t.Fatalf("cluster spec version = %q, want %q", cluster.Spec.Version, "2.6.0")
	}
	if cluster.Spec.Replicas != 3 {
		t.Fatalf("cluster spec replicas = %d, want %d", cluster.Spec.Replicas, 3)
	}
	if cluster.Spec.Profile != openbaov1alpha1.ProfileDevelopment {
		t.Fatalf("cluster spec profile = %q, want %q", cluster.Spec.Profile, openbaov1alpha1.ProfileDevelopment)
	}
	if cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled {
		t.Fatalf("cluster selfInit = %#v, want enabled", cluster.Spec.SelfInit)
	}
	if cluster.Spec.SelfInit.OIDC == nil || !cluster.Spec.SelfInit.OIDC.Enabled || cluster.Spec.SelfInit.OIDC.Audience != "openbao-operator" {
		t.Fatalf("cluster selfInit oidc = %#v, want operator audience", cluster.Spec.SelfInit.OIDC)
	}
	foundSecretMount := false
	for _, request := range cluster.Spec.SelfInit.Requests {
		if request.Path == "sys/mounts/secret" {
			foundSecretMount = true
			break
		}
	}
	if !foundSecretMount {
		t.Fatalf("cluster selfInit requests = %#v, want secret mount request", cluster.Spec.SelfInit.Requests)
	}
	if cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas != 1 {
		t.Fatalf("cluster readReplicas = %#v, want one read replica", cluster.Spec.ReadReplicas)
	}
	if cluster.Spec.Storage.Size != "20Gi" {
		t.Fatalf("cluster storage size = %q, want %q", cluster.Spec.Storage.Size, "20Gi")
	}
	if cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyRollingUpdate || cluster.Spec.Upgrade.PreUpgradeSnapshot {
		t.Fatalf("cluster upgrade = %#v, want rolling update without pre-upgrade snapshot", cluster.Spec.Upgrade)
	}
}

func assertProjectedLocalClusterWithGateway(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedLocalCluster(t, claim, cluster)
	if cluster.Spec.Ingress != nil {
		t.Fatalf("cluster spec ingress = %#v, want nil for gateway exposure", cluster.Spec.Ingress)
	}
	if cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		t.Fatalf("cluster spec gateway = %#v, want enabled gateway", cluster.Spec.Gateway)
	}
	if cluster.Spec.Gateway.GatewayRef.Name != "internal-gateway" || cluster.Spec.Gateway.GatewayRef.Namespace != "networking" {
		t.Fatalf("cluster spec gatewayRef = %#v, want rendered gateway ref", cluster.Spec.Gateway.GatewayRef)
	}
	if cluster.Spec.Gateway.ListenerName != "" {
		t.Fatalf("cluster spec listenerName = %q, want empty", cluster.Spec.Gateway.ListenerName)
	}
	if cluster.Spec.Gateway.Hostname != "payments-bao.example.internal" {
		t.Fatalf("cluster spec hostname = %q, want payments-bao.example.internal", cluster.Spec.Gateway.Hostname)
	}
	if cluster.Spec.Gateway.Path != "/" {
		t.Fatalf("cluster spec path = %q, want /", cluster.Spec.Gateway.Path)
	}
	if cluster.Spec.Gateway.BackendTLS == nil || cluster.Spec.Gateway.BackendTLS.Enabled == nil || !*cluster.Spec.Gateway.BackendTLS.Enabled {
		t.Fatalf("cluster spec backendTLS = %#v, want enabled", cluster.Spec.Gateway.BackendTLS)
	}
}

func assertProjectedLocalClusterWithIngress(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedLocalCluster(t, claim, cluster)
	if cluster.Spec.Gateway != nil {
		t.Fatalf("cluster spec gateway = %#v, want nil for ingress exposure", cluster.Spec.Gateway)
	}
	if cluster.Spec.Ingress == nil || !cluster.Spec.Ingress.Enabled {
		t.Fatalf("cluster spec ingress = %#v, want enabled ingress", cluster.Spec.Ingress)
	}
	if cluster.Spec.Ingress.ClassName == nil || *cluster.Spec.Ingress.ClassName != "nginx" {
		t.Fatalf("cluster spec ingress class = %#v, want nginx", cluster.Spec.Ingress.ClassName)
	}
	if cluster.Spec.Ingress.Host != "payments-bao.example.internal" {
		t.Fatalf("cluster spec ingress host = %q, want payments-bao.example.internal", cluster.Spec.Ingress.Host)
	}
	if cluster.Spec.Ingress.Path != "/" {
		t.Fatalf("cluster spec ingress path = %q, want /", cluster.Spec.Ingress.Path)
	}
	if cluster.Spec.Ingress.PathType != openbaov1alpha1.IngressPathTypePrefix {
		t.Fatalf("cluster spec ingress pathType = %q, want Prefix", cluster.Spec.Ingress.PathType)
	}
	if cluster.Spec.Ingress.ReadinessMode != openbaov1alpha1.IngressReadinessModeLoadBalancerPublished {
		t.Fatalf("cluster spec ingress readinessMode = %q, want LoadBalancerPublished", cluster.Spec.Ingress.ReadinessMode)
	}
	if cluster.Spec.Ingress.Annotations["nginx.ingress.kubernetes.io/backend-protocol"] != "HTTPS" {
		t.Fatalf("cluster spec ingress annotations = %#v, want backend TLS annotation", cluster.Spec.Ingress.Annotations)
	}
}

func assertProjectedHardenedLocalCluster(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	if cluster.Labels[constants.LabelOpenBaoOwnershipMode] != constants.LabelValueOpenBaoOwnershipClaimManaged {
		t.Fatalf("ownership mode label = %q, want %q", cluster.Labels[constants.LabelOpenBaoOwnershipMode], constants.LabelValueOpenBaoOwnershipClaimManaged)
	}
	if cluster.Labels[constants.LabelOpenBaoClaimNamespace] != claim.Namespace {
		t.Fatalf("claim namespace label = %q, want %q", cluster.Labels[constants.LabelOpenBaoClaimNamespace], claim.Namespace)
	}
	if cluster.Labels[constants.LabelOpenBaoClaimName] != claim.Name {
		t.Fatalf("claim name label = %q, want %q", cluster.Labels[constants.LabelOpenBaoClaimName], claim.Name)
	}
	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		t.Fatalf("cluster spec profile = %q, want %q", cluster.Spec.Profile, openbaov1alpha1.ProfileHardened)
	}
	if cluster.Spec.TLS.Mode != openbaov1alpha1.TLSModeExternal {
		t.Fatalf("cluster spec tls mode = %q, want %q", cluster.Spec.TLS.Mode, openbaov1alpha1.TLSModeExternal)
	}
	if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Type != "transit" {
		t.Fatalf("cluster spec unseal = %#v, want transit", cluster.Spec.Unseal)
	}
	if cluster.Spec.Unseal.Transit == nil || cluster.Spec.Unseal.Transit.Address != "https://transit.example.internal:8200" {
		t.Fatalf("cluster spec transit = %#v, want rendered transit config", cluster.Spec.Unseal.Transit)
	}
	if cluster.Spec.Unseal.Transit.TLSCACert != "/etc/bao/seal-creds/ca.crt" {
		t.Fatalf("cluster spec transit tlsCACert = %q, want mounted transit CA path", cluster.Spec.Unseal.Transit.TLSCACert)
	}
	if cluster.Spec.Unseal.CredentialsSecretRef == nil || cluster.Spec.Unseal.CredentialsSecretRef.Name != "transit-unseal-creds" {
		t.Fatalf("cluster spec credentialsSecretRef = %#v, want transit-unseal-creds", cluster.Spec.Unseal.CredentialsSecretRef)
	}
}

func assertProjectedLocalClusterWithAuthConfig(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedLocalCluster(t, claim, cluster)
	if len(cluster.Spec.SelfInit.Requests) != 2 {
		t.Fatalf("cluster selfInit requests = %#v, want engine + auth enable requests", cluster.Spec.SelfInit.Requests)
	}
	var foundEnable bool
	for _, request := range cluster.Spec.SelfInit.Requests {
		switch request.Path {
		case "sys/auth/kubernetes":
			foundEnable = true
			if request.AuthMethod == nil || request.AuthMethod.ConfigFromRef == nil {
				t.Fatalf("auth enable request = %#v, want configFromRef-backed auth method", request.AuthMethod)
			}
		}
	}
	if !foundEnable {
		t.Fatalf("cluster selfInit requests = %#v, want sys/auth/kubernetes request", cluster.Spec.SelfInit.Requests)
	}
}

func assertProjectedLocalClusterWithPolicyBundle(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedLocalCluster(t, claim, cluster)
	var foundPolicy bool
	for _, request := range cluster.Spec.SelfInit.Requests {
		if request.Path != "sys/policies/acl/app-readwrite" {
			continue
		}
		foundPolicy = true
		if request.Policy == nil || request.Policy.ContentFromRef == nil {
			t.Fatalf("policy request = %#v, want contentFromRef-backed policy", request.Policy)
		}
	}
	if !foundPolicy {
		t.Fatalf("cluster selfInit requests = %#v, want sys/policies/acl/app-readwrite request", cluster.Spec.SelfInit.Requests)
	}
}

func assertProjectedLocalClusterWithAuditDevice(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedLocalCluster(t, claim, cluster)
	if len(cluster.Spec.Audit) != 1 {
		t.Fatalf("cluster spec.audit = %#v, want one declarative audit device", cluster.Spec.Audit)
	}
	audit := cluster.Spec.Audit[0]
	if audit.Type != "file" || audit.Path != "stdout" {
		t.Fatalf("audit device = %#v, want file/stdout declarative audit", audit)
	}
	if audit.FileOptions == nil || audit.FileOptions.FilePath != "stdout" {
		t.Fatalf("audit fileOptions = %#v, want stdout file path", audit.FileOptions)
	}
}

func assertProjectedBootstrapRefsExist(t *testing.T, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()

	for _, request := range cluster.Spec.SelfInit.Requests {
		if request.AuthMethod != nil && request.AuthMethod.ConfigFromRef != nil {
			assertBootstrapProjectedRefExists(t, c, cluster.Namespace, *request.AuthMethod.ConfigFromRef)
		}
		if request.Policy != nil && request.Policy.ContentFromRef != nil {
			assertBootstrapProjectedRefExists(t, c, cluster.Namespace, *request.Policy.ContentFromRef)
		}
		if request.AuditDevice != nil && request.AuditDevice.SinkFromRef != nil {
			assertBootstrapProjectedRefExists(t, c, cluster.Namespace, *request.AuditDevice.SinkFromRef)
		}
	}
}

func assertBootstrapProjectedRefExists(
	t *testing.T,
	c client.Client,
	namespace string,
	ref openbaov1alpha1.TypedObjectReference,
) {
	t.Helper()

	key := client.ObjectKey{Namespace: namespace, Name: ref.Name}
	switch ref.Kind {
	case kindConfigMap:
		obj := &corev1.ConfigMap{}
		if err := c.Get(context.Background(), key, obj); err != nil {
			t.Fatalf("Get projected ConfigMap %s/%s error = %v", key.Namespace, key.Name, err)
		}
	case kindSecret:
		obj := &corev1.Secret{}
		if err := c.Get(context.Background(), key, obj); err != nil {
			t.Fatalf("Get projected Secret %s/%s error = %v", key.Namespace, key.Name, err)
		}
	default:
		t.Fatalf("unexpected projected bootstrap ref kind %q", ref.Kind)
	}
}

func assertProjectedLocalClusterWithBackup(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedLocalCluster(t, claim, cluster)
	if cluster.Spec.Backup == nil {
		t.Fatal("cluster spec backup = nil, want projected backup config")
	}
	if cluster.Spec.Backup.Schedule != "0 3 * * *" {
		t.Fatalf("cluster backup schedule = %q, want %q", cluster.Spec.Backup.Schedule, "0 3 * * *")
	}
	if cluster.Spec.Backup.JWTAuthRole != "openbao-operator-backup" {
		t.Fatalf("cluster backup jwtAuthRole = %q, want %q", cluster.Spec.Backup.JWTAuthRole, "openbao-operator-backup")
	}
	if cluster.Spec.Backup.Target.Provider != "s3" {
		t.Fatalf("cluster backup provider = %q, want %q", cluster.Spec.Backup.Target.Provider, "s3")
	}
	if cluster.Spec.Backup.Target.Bucket != "payments-prod" {
		t.Fatalf("cluster backup bucket = %q, want %q", cluster.Spec.Backup.Target.Bucket, "payments-prod")
	}
	if cluster.Spec.Backup.Target.PathPrefix != "claims/payments/payments-bao/finance" {
		t.Fatalf("cluster backup pathPrefix = %q, want %q", cluster.Spec.Backup.Target.PathPrefix, "claims/payments/payments-bao/finance")
	}
	if cluster.Spec.Backup.Target.RoleARN != "arn:aws:iam::123456789012:role/openbao-backup" {
		t.Fatalf("cluster backup roleArn = %q, want projected role", cluster.Spec.Backup.Target.RoleARN)
	}
	if cluster.Spec.Backup.Target.WorkloadIdentity == nil || cluster.Spec.Backup.Target.WorkloadIdentity.ServiceAccountAnnotations["eks.amazonaws.com/role-arn"] == "" {
		t.Fatalf("cluster backup workloadIdentity = %#v, want projected workload identity", cluster.Spec.Backup.Target.WorkloadIdentity)
	}
	if cluster.Spec.Backup.Target.PartSize != 16777216 || cluster.Spec.Backup.Target.Concurrency != 5 {
		t.Fatalf("cluster backup target = %#v, want rendered transfer settings", cluster.Spec.Backup.Target)
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) != 1 {
		t.Fatalf("cluster network = %#v, want rendered backup egress rule", cluster.Spec.Network)
	}
	if cluster.Spec.Network.EgressRules[0].To[0].IPBlock == nil || cluster.Spec.Network.EgressRules[0].To[0].IPBlock.CIDR != "10.10.0.0/16" {
		t.Fatalf("cluster network egress destination = %#v, want rendered ipBlock", cluster.Spec.Network.EgressRules)
	}
	if len(cluster.Spec.Network.EgressRules[0].Ports) != 1 || cluster.Spec.Network.EgressRules[0].Ports[0].Port == nil || cluster.Spec.Network.EgressRules[0].Ports[0].Port.IntVal != 443 {
		t.Fatalf("cluster network egress ports = %#v, want rendered tcp/443", cluster.Spec.Network.EgressRules)
	}
}

func assertProjectedHardenedLocalClusterWithBackup(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	t.Helper()

	assertProjectedHardenedLocalCluster(t, claim, cluster)
	if cluster.Spec.Backup == nil {
		t.Fatal("cluster spec backup = nil, want projected backup config")
	}
	if cluster.Spec.Backup.Schedule != "0 3 * * *" {
		t.Fatalf("cluster backup schedule = %q, want %q", cluster.Spec.Backup.Schedule, "0 3 * * *")
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) != 1 {
		t.Fatalf("cluster network = %#v, want rendered backup egress rule", cluster.Spec.Network)
	}
}
