package infra

import (
	"context"
	"os"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
)

func TestSentinelImage(t *testing.T) {
	tests := []struct {
		name           string
		cluster        *openbaov1alpha1.OpenBaoCluster
		envVersion     string
		wantImage      string
		wantWarningLog bool
	}{
		{
			name: "uses custom image when specified",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Sentinel: &openbaov1alpha1.SentinelConfig{
						Image: "custom-registry/sentinel:v1.0.0",
					},
				},
			},
			envVersion:     "",
			wantImage:      "custom-registry/sentinel:v1.0.0",
			wantWarningLog: false,
		},
		{
			name: "derives from operator version when env var set",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Sentinel: &openbaov1alpha1.SentinelConfig{},
				},
			},
			envVersion:     "v2.1.0",
			wantImage:      constants.DefaultSentinelImageRepository + ":v2.1.0",
			wantWarningLog: false,
		},
		{
			name: "falls back to latest when no version",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Sentinel: &openbaov1alpha1.SentinelConfig{},
				},
			},
			envVersion:     "",
			wantImage:      constants.DefaultSentinelImageRepository + ":latest",
			wantWarningLog: true,
		},
		{
			name: "uses custom image even when env var set",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Sentinel: &openbaov1alpha1.SentinelConfig{
						Image: "custom-registry/sentinel:v1.0.0",
					},
				},
			},
			envVersion:     "v2.1.0",
			wantImage:      "custom-registry/sentinel:v1.0.0",
			wantWarningLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env var
			originalEnv := os.Getenv(constants.EnvOperatorVersion)
			defer func() {
				if originalEnv != "" {
					_ = os.Setenv(constants.EnvOperatorVersion, originalEnv)
				} else {
					_ = os.Unsetenv(constants.EnvOperatorVersion)
				}
			}()

			if tt.envVersion != "" {
				_ = os.Setenv(constants.EnvOperatorVersion, tt.envVersion)
			} else {
				_ = os.Unsetenv(constants.EnvOperatorVersion)
			}

			got := SentinelImage(tt.cluster, logr.Discard())
			if got != tt.wantImage {
				t.Errorf("SentinelImage() = %v, want %v", got, tt.wantImage)
			}
		})
	}
}

func TestEnsureSentinelServiceAccount(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("sentinel-sa", "default")
	cluster.Spec.Sentinel = &openbaov1alpha1.SentinelConfig{
		Enabled: true,
	}

	ctx := context.Background()

	if err := manager.ensureSentinelServiceAccount(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelServiceAccount() error = %v", err)
	}

	sa := &corev1.ServiceAccount{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      constants.SentinelServiceAccountName,
	}, sa)
	if err != nil {
		t.Fatalf("expected Sentinel ServiceAccount to exist: %v", err)
	}

	// Verify labels
	expectedLabels := map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelAppComponent:   "sentinel",
		constants.LabelOpenBaoCluster: cluster.Name,
	}
	for key, expectedVal := range expectedLabels {
		if sa.Labels[key] != expectedVal {
			t.Errorf("expected ServiceAccount label %s=%s, got %s", key, expectedVal, sa.Labels[key])
		}
	}
}

//nolint:gocyclo // Table-driven test with multiple assertions
func TestEnsureSentinelRBAC(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("sentinel-rbac", "default")
	cluster.Spec.Sentinel = &openbaov1alpha1.SentinelConfig{
		Enabled: true,
	}

	ctx := context.Background()

	if err := manager.ensureSentinelRBAC(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelRBAC() error = %v", err)
	}

	// Verify Role exists
	role := &rbacv1.Role{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      constants.SentinelRoleName,
	}, role)
	if err != nil {
		t.Fatalf("expected Sentinel Role to exist: %v", err)
	}

	// Verify Role has correct rules
	if len(role.Rules) == 0 {
		t.Fatalf("expected Sentinel Role to have rules")
	}

	// Verify Role has read-only access to infrastructure
	// The role has 3 separate rules: StatefulSets (apps), Services/ConfigMaps (core), and OpenBaoClusters
	hasStatefulSetRule := false
	hasServiceConfigMapRule := false
	hasOpenBaoClusterRule := false
	for _, rule := range role.Rules {
		// Check for StatefulSet read rule (apps API group)
		if contains(rule.APIGroups, "apps") &&
			contains(rule.Resources, "statefulsets") &&
			!contains(rule.Resources, "deployments") &&
			contains(rule.Verbs, "get") &&
			contains(rule.Verbs, "list") &&
			contains(rule.Verbs, "watch") {
			hasStatefulSetRule = true
		}
		// Check for Service/ConfigMap read rule (core API group)
		if contains(rule.APIGroups, "") &&
			contains(rule.Resources, "services") &&
			contains(rule.Resources, "configmaps") &&
			contains(rule.Verbs, "get") &&
			contains(rule.Verbs, "list") &&
			contains(rule.Verbs, "watch") {
			hasServiceConfigMapRule = true
		}
		// Check for OpenBaoCluster patch rule
		if contains(rule.APIGroups, "openbao.org") &&
			contains(rule.Resources, "openbaoclusters") &&
			contains(rule.Verbs, "get") &&
			contains(rule.Verbs, "list") &&
			contains(rule.Verbs, "patch") &&
			contains(rule.Verbs, "watch") {
			hasOpenBaoClusterRule = true
		}
	}

	if !hasStatefulSetRule {
		t.Error("expected Sentinel Role to have StatefulSet read rule")
	}
	if !hasServiceConfigMapRule {
		t.Error("expected Sentinel Role to have Service/ConfigMap read rule")
	}
	if !hasOpenBaoClusterRule {
		t.Error("expected Sentinel Role to have OpenBaoCluster patch rule")
	}

	// Verify RoleBinding exists
	roleBinding := &rbacv1.RoleBinding{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      constants.SentinelRoleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected Sentinel RoleBinding to exist: %v", err)
	}

	// Verify RoleBinding references the Role
	if roleBinding.RoleRef.Name != constants.SentinelRoleName {
		t.Fatalf("expected RoleBinding to reference Role %q, got %q", constants.SentinelRoleName, roleBinding.RoleRef.Name)
	}

	// Verify RoleBinding references the Sentinel ServiceAccount
	if len(roleBinding.Subjects) == 0 {
		t.Fatalf("expected RoleBinding to have subjects")
	}

	foundSubject := false
	for _, subject := range roleBinding.Subjects {
		if subject.Kind == "ServiceAccount" && subject.Name == constants.SentinelServiceAccountName && subject.Namespace == cluster.Namespace {
			foundSubject = true
			break
		}
	}
	if !foundSubject {
		t.Fatalf("expected RoleBinding to reference ServiceAccount %q", constants.SentinelServiceAccountName)
	}
}

func TestEnsureSentinelDeployment(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("sentinel-deploy", "default")
	cluster.Spec.Sentinel = &openbaov1alpha1.SentinelConfig{
		Enabled:               true,
		DebounceWindowSeconds: func() *int32 { v := int32(5); return &v }(),
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("200m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("256Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
	}

	ctx := context.Background()

	// Create ServiceAccount first (required for Deployment)
	if err := manager.ensureSentinelServiceAccount(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelServiceAccount() error = %v", err)
	}

	if err := manager.ensureSentinelDeployment(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("ensureSentinelDeployment() error = %v", err)
	}

	deploymentName := cluster.Name + constants.SentinelDeploymentNameSuffix
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      deploymentName,
	}, deployment)
	if err != nil {
		t.Fatalf("expected Sentinel Deployment to exist: %v", err)
	}

	// Verify Deployment spec
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Errorf("expected Deployment to have 1 replica, got %v", deployment.Spec.Replicas)
	}

	// Verify container
	if len(deployment.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected Deployment to have 1 container, got %d", len(deployment.Spec.Template.Spec.Containers))
	}

	container := deployment.Spec.Template.Spec.Containers[0]
	if container.Name != "sentinel" {
		t.Errorf("expected container name to be 'sentinel', got %q", container.Name)
	}

	// Verify environment variables
	envVars := make(map[string]string)
	for _, env := range container.Env {
		if env.Value != "" {
			envVars[env.Name] = env.Value
		}
	}

	if envVars[constants.EnvClusterName] != cluster.Name {
		t.Errorf("expected CLUSTER_NAME env var to be %q, got %q", cluster.Name, envVars[constants.EnvClusterName])
	}

	if envVars[constants.EnvSentinelDebounceWindowSeconds] != "5" {
		t.Errorf("expected SENTINEL_DEBOUNCE_WINDOW_SECONDS env var to be '5', got %q", envVars[constants.EnvSentinelDebounceWindowSeconds])
	}

	// Verify resources
	if container.Resources.Requests.Memory().Value() != cluster.Spec.Sentinel.Resources.Requests.Memory().Value() {
		t.Errorf("expected memory request to match cluster spec")
	}

	// Verify probes
	if container.LivenessProbe == nil {
		t.Error("expected container to have liveness probe")
	}
	if container.ReadinessProbe == nil {
		t.Error("expected container to have readiness probe")
	}

	// Verify Pod security context
	if deployment.Spec.Template.Spec.SecurityContext == nil {
		t.Error("expected Deployment to have PodSecurityContext")
	}
	if deployment.Spec.Template.Spec.SecurityContext.RunAsUser == nil || *deployment.Spec.Template.Spec.SecurityContext.RunAsUser != constants.UserNonRoot {
		t.Errorf("expected PodSecurityContext.RunAsUser to be %d, got %v", constants.UserNonRoot, deployment.Spec.Template.Spec.SecurityContext.RunAsUser)
	}
	if deployment.Spec.Template.Spec.SecurityContext.SeccompProfile == nil || deployment.Spec.Template.Spec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected PodSecurityContext.SeccompProfile.Type to be RuntimeDefault, got %v", deployment.Spec.Template.Spec.SecurityContext.SeccompProfile)
	}

	// Verify container SecurityContext (required for restricted Pod Security Standard)
	if container.SecurityContext == nil {
		t.Fatal("expected container to have SecurityContext")
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation != false {
		t.Errorf("expected container SecurityContext.AllowPrivilegeEscalation to be false, got %v", container.SecurityContext.AllowPrivilegeEscalation)
	}
	if container.SecurityContext.Capabilities == nil || len(container.SecurityContext.Capabilities.Drop) == 0 {
		t.Fatal("expected container SecurityContext.Capabilities.Drop to be set")
	}
	foundAllCapability := false
	for _, cap := range container.SecurityContext.Capabilities.Drop {
		if cap == "ALL" {
			foundAllCapability = true
			break
		}
	}
	if !foundAllCapability {
		t.Errorf("expected container SecurityContext.Capabilities.Drop to include 'ALL', got %v", container.SecurityContext.Capabilities.Drop)
	}
	if container.SecurityContext.SeccompProfile == nil || container.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("expected container SecurityContext.SeccompProfile.Type to be RuntimeDefault, got %v", container.SecurityContext.SeccompProfile)
	}
}

func TestEnsureSentinelDeployment_DefaultResources(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("sentinel-deploy-defaults", "default")
	cluster.Spec.Sentinel = &openbaov1alpha1.SentinelConfig{
		Enabled: true,
		// Resources not specified, should use defaults
	}

	ctx := context.Background()

	// Create ServiceAccount first
	if err := manager.ensureSentinelServiceAccount(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelServiceAccount() error = %v", err)
	}

	if err := manager.ensureSentinelDeployment(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("ensureSentinelDeployment() error = %v", err)
	}

	deploymentName := cluster.Name + constants.SentinelDeploymentNameSuffix
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      deploymentName,
	}, deployment)
	if err != nil {
		t.Fatalf("expected Sentinel Deployment to exist: %v", err)
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify default resources
	expectedMemoryRequest := resource.MustParse("64Mi")
	expectedCPURequest := resource.MustParse("100m")
	expectedMemoryLimit := resource.MustParse("128Mi")
	expectedCPULimit := resource.MustParse("200m")

	if container.Resources.Requests.Memory().Value() != expectedMemoryRequest.Value() {
		t.Errorf("expected default memory request to be %v, got %v", expectedMemoryRequest, container.Resources.Requests.Memory())
	}
	if container.Resources.Requests.Cpu().Value() != expectedCPURequest.Value() {
		t.Errorf("expected default CPU request to be %v, got %v", expectedCPURequest, container.Resources.Requests.Cpu())
	}
	if container.Resources.Limits.Memory().Value() != expectedMemoryLimit.Value() {
		t.Errorf("expected default memory limit to be %v, got %v", expectedMemoryLimit, container.Resources.Limits.Memory())
	}
	if container.Resources.Limits.Cpu().Value() != expectedCPULimit.Value() {
		t.Errorf("expected default CPU limit to be %v, got %v", expectedCPULimit, container.Resources.Limits.Cpu())
	}
}

func TestEnsureSentinelCleanup(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil)

	cluster := newMinimalCluster("sentinel-cleanup", "default")
	cluster.Spec.Sentinel = &openbaov1alpha1.SentinelConfig{
		Enabled: true,
	}

	ctx := context.Background()

	// Create Sentinel resources first
	if err := manager.ensureSentinelServiceAccount(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelServiceAccount() error = %v", err)
	}
	if err := manager.ensureSentinelRBAC(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelRBAC() error = %v", err)
	}
	if err := manager.ensureSentinelDeployment(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("ensureSentinelDeployment() error = %v", err)
	}

	// Cleanup should delete all resources
	if err := manager.ensureSentinelCleanup(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureSentinelCleanup() error = %v", err)
	}

	// Verify Deployment is deleted
	deploymentName := cluster.Name + constants.SentinelDeploymentNameSuffix
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      deploymentName,
	}, &appsv1.Deployment{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected Deployment to be deleted, got error: %v", err)
	}

	// Verify ServiceAccount is deleted
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      constants.SentinelServiceAccountName,
	}, &corev1.ServiceAccount{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected ServiceAccount to be deleted, got error: %v", err)
	}

	// Verify Role is deleted
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      constants.SentinelRoleName,
	}, &rbacv1.Role{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected Role to be deleted, got error: %v", err)
	}

	// Verify RoleBinding is deleted
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      constants.SentinelRoleBindingName,
	}, &rbacv1.RoleBinding{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected RoleBinding to be deleted, got error: %v", err)
	}
}

// Helper function to check if a slice contains a value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
