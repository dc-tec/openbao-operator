//go:build integration
// +build integration

package integration

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

func TestKustomizeClusterScopedResourcesHaveNoNamespace(t *testing.T) {
	testCases := []struct {
		name string
		dir  string
	}{
		{
			name: "config-default",
			dir:  filepath.Join("..", "..", "config", "default"),
		},
		{
			name: "config-overlays-single-tenant",
			dir:  filepath.Join("..", "..", "config", "overlays", "single-tenant"),
		},
		{
			name: "config-overlays-single-tenant-custom-identity",
			dir:  filepath.Join("..", "..", "config", "overlays", "single-tenant-custom-identity"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlBytes := kustomizeBuild(t, tc.dir)
			decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), 4096)

			for {
				var raw map[string]any
				if err := decoder.Decode(&raw); err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					t.Fatalf("decode YAML: %v", err)
				}
				if len(raw) == 0 {
					continue
				}

				obj := &unstructured.Unstructured{Object: raw}
				if obj.GetAPIVersion() == "" || obj.GetKind() == "" || obj.GetName() == "" {
					continue
				}

				if !isClusterScopedManifestObject(obj.GroupVersionKind()) {
					continue
				}

				if (tc.name == "config-overlays-single-tenant" || tc.name == "config-overlays-single-tenant-custom-identity") &&
					allowsClusterScopedNamespaceInSingleTenantOverlay(obj.GroupVersionKind()) {
					continue
				}

				if obj.GetNamespace() != "" {
					t.Fatalf("cluster-scoped %s %s has unexpected namespace %q", obj.GetKind(), obj.GetName(), obj.GetNamespace())
				}
			}
		})
	}
}

func allowsClusterScopedNamespaceInSingleTenantOverlay(gvk schema.GroupVersionKind) bool {
	return gvk.Group == testAdmissionRegistrationGroup &&
		(gvk.Kind == testKindVAP || gvk.Kind == testKindVAPBinding)
}

func TestKustomizePolicy_BindingsReferenceExistingPolicies(t *testing.T) {
	testCases := []struct {
		name string
		dir  string
	}{
		{
			name: "config-policy",
			dir:  filepath.Join("..", "..", "config", "policy"),
		},
		{
			name: "config-default",
			dir:  filepath.Join("..", "..", "config", "default"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlBytes := kustomizeBuild(t, tc.dir)
			objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
				gvk := u.GroupVersionKind()
				return gvk.Group == testAdmissionRegistrationGroup &&
					(gvk.Kind == testKindVAP || gvk.Kind == testKindVAPBinding)
			})

			policies := make(map[string]struct{})
			bindings := make(map[string]string)
			for _, obj := range objs {
				switch obj.GetKind() {
				case testKindVAP:
					policies[obj.GetName()] = struct{}{}
				case testKindVAPBinding:
					policyName, found, err := unstructured.NestedString(obj.Object, "spec", "policyName")
					if err != nil {
						t.Fatalf("read spec.policyName for binding %s: %v", obj.GetName(), err)
					}
					if !found || policyName == "" {
						t.Fatalf("binding %s has empty spec.policyName", obj.GetName())
					}
					bindings[obj.GetName()] = policyName
				}
			}

			if len(bindings) == 0 {
				t.Fatal("expected at least one ValidatingAdmissionPolicyBinding")
			}

			for bindingName, policyName := range bindings {
				if _, ok := policies[policyName]; !ok {
					t.Fatalf("binding %s references missing policy %s", bindingName, policyName)
				}
			}
		})
	}
}

func TestKustomizeDefault_LockManagedPolicyRequiresOpenBaoLabels(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		gvk := u.GroupVersionKind()
		return gvk.Group == testAdmissionRegistrationGroup &&
			gvk.Kind == testKindVAP &&
			strings.HasSuffix(u.GetName(), "openbao-lock-managed-resource-mutations")
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one openbao-lock-managed-resource-mutations policy, got %d", len(objs))
	}

	variables, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "variables")
	if err != nil {
		t.Fatalf("read spec.variables: %v", err)
	}
	if !found {
		t.Fatal("openbao-lock-managed-resource-mutations policy missing spec.variables")
	}

	var hasOpenBaoLabelExpression string
	var maintenanceAuthorizedExpression string
	var maintenanceClusterNameExpression string
	var isManagedExpression string
	for _, variable := range variables {
		variableMap, ok := variable.(map[string]any)
		if !ok {
			continue
		}
		name, _ := variableMap["name"].(string)
		expression, _ := variableMap["expression"].(string)
		switch name {
		case "has_openbao_specific_label":
			hasOpenBaoLabelExpression = expression
		case "maintenance_authorized":
			maintenanceAuthorizedExpression = expression
		case "maintenance_cluster_name":
			maintenanceClusterNameExpression = expression
		case "is_managed":
			isManagedExpression = expression
		}
	}

	if !strings.Contains(hasOpenBaoLabelExpression, `k.startsWith("openbao.org/")`) {
		t.Fatalf(
			"has_openbao_specific_label expression does not enforce openbao.org/* label gate: %q",
			hasOpenBaoLabelExpression,
		)
	}
	if !strings.Contains(isManagedExpression, "variables.has_openbao_specific_label") {
		t.Fatalf("is_managed expression does not require has_openbao_specific_label: %q", isManagedExpression)
	}
	if !strings.Contains(maintenanceClusterNameExpression, `"openbao.org/cluster"`) {
		t.Fatalf(
			"maintenance_cluster_name expression does not prefer openbao.org/cluster: %q",
			maintenanceClusterNameExpression,
		)
	}
	if !strings.Contains(maintenanceClusterNameExpression, `"app.kubernetes.io/instance"`) {
		t.Fatalf(
			"maintenance_cluster_name expression does not fall back to app.kubernetes.io/instance: %q",
			maintenanceClusterNameExpression,
		)
	}
	if !strings.Contains(maintenanceAuthorizedExpression, `authorizer.group("openbao.org")`) {
		t.Fatalf("maintenance_authorized expression does not use the CEL authorizer: %q", maintenanceAuthorizedExpression)
	}
	if !strings.Contains(maintenanceAuthorizedExpression, `check("maintenance")`) {
		t.Fatalf(
			"maintenance_authorized expression does not check the custom maintenance verb: %q",
			maintenanceAuthorizedExpression,
		)
	}
}

func TestKustomizeDefault_OpenBaoClusterPolicyBlocksUpgradeStrategySwitches(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		gvk := u.GroupVersionKind()
		return gvk.Group == testAdmissionRegistrationGroup &&
			gvk.Kind == testKindVAP &&
			strings.HasSuffix(u.GetName(), "openbao-validate-openbaocluster")
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one openbao-validate-openbaocluster policy, got %d", len(objs))
	}

	variables, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "variables")
	if err != nil || !found {
		t.Fatalf("read policy variables: found=%v err=%v", found, err)
	}

	var hasRequestedStrategy bool
	var hasPreviousStrategy bool
	for _, variable := range variables {
		variableMap, ok := variable.(map[string]any)
		if !ok {
			continue
		}
		name, _ := variableMap["name"].(string)
		switch name {
		case "requested_upgrade_strategy":
			hasRequestedStrategy = true
		case "previous_upgrade_strategy":
			hasPreviousStrategy = true
		}
	}

	if !hasRequestedStrategy || !hasPreviousStrategy {
		t.Fatalf(
			"expected strategy transition variables in openbao-validate-openbaocluster policy, got requested=%v previous=%v",
			hasRequestedStrategy,
			hasPreviousStrategy,
		)
	}

	validations, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "validations")
	if err != nil || !found {
		t.Fatalf("read policy validations: found=%v err=%v", found, err)
	}

	const wantMessage = "spec.upgrade.strategy is immutable after creation; " +
		"switching between RollingUpdate and BlueGreen is not supported."
	var foundRule bool
	for _, validation := range validations {
		validationMap, ok := validation.(map[string]any)
		if !ok {
			continue
		}
		message, _ := validationMap["message"].(string)
		expression, _ := validationMap["expression"].(string)
		if message == wantMessage &&
			strings.Contains(expression, "variables.requested_upgrade_strategy") &&
			strings.Contains(expression, "variables.previous_upgrade_strategy") {
			foundRule = true
			break
		}
	}

	if !foundRule {
		t.Fatalf("openbao-validate-openbaocluster policy is missing the upgrade strategy immutability rule")
	}
}

func TestKustomizeDefault_OpenBaoClusterPolicyProtectsTransitUnseal(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		gvk := u.GroupVersionKind()
		return gvk.Group == testAdmissionRegistrationGroup &&
			gvk.Kind == testKindVAP &&
			strings.HasSuffix(u.GetName(), "openbao-validate-openbaocluster")
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one openbao-validate-openbaocluster policy, got %d", len(objs))
	}

	validations, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "validations")
	if err != nil || !found {
		t.Fatalf("read policy validations: found=%v err=%v", found, err)
	}

	var foundHTTPS bool
	var foundUnsafeURLComponents bool
	var foundSecretAuthorizer bool
	var foundBackupSecretAuthorizer bool
	var foundSystemSecretBlock bool
	for _, validation := range validations {
		validationMap, ok := validation.(map[string]any)
		if !ok {
			continue
		}
		message, _ := validationMap["message"].(string)
		expression, _ := validationMap["expression"].(string)
		switch {
		case message == "Transit unseal address must use HTTPS." &&
			strings.Contains(expression, `object.spec.unseal.transit.address.startsWith("https://")`):
			foundHTTPS = true
		case strings.Contains(message, "must not include userinfo") &&
			strings.Contains(expression, `contains("@")`) &&
			strings.Contains(expression, `169\\.254`) &&
			strings.Contains(expression, `[fe80:`):
			foundUnsafeURLComponents = true
		case strings.Contains(message, "Users configuring unseal credentials") &&
			strings.Contains(expression, `authorizer.group("")`) &&
			strings.Contains(expression, `resource("secrets")`) &&
			strings.Contains(expression, `check("get")`) &&
			!strings.Contains(expression, `object.spec.unseal.type != "transit"`):
			foundSecretAuthorizer = true
		case strings.Contains(message, "Users configuring backup credentials") &&
			strings.Contains(expression, `authorizer.group("")`) &&
			strings.Contains(expression, `resource("secrets")`) &&
			strings.Contains(expression, `check("get")`) &&
			strings.Contains(expression, `object.spec.backup.target.credentialsSecretRef`) &&
			strings.Contains(expression, `object.spec.backup.tokenSecretRef`):
			foundBackupSecretAuthorizer = true
		case strings.Contains(message, "system secrets") &&
			strings.Contains(expression, "object.spec.unseal.credentialsSecretRef") &&
			strings.Contains(expression, "root-token"):
			foundSystemSecretBlock = true
		}
	}

	if !foundHTTPS || !foundUnsafeURLComponents || !foundSecretAuthorizer || !foundBackupSecretAuthorizer || !foundSystemSecretBlock {
		t.Fatalf(
			"openbao-validate-openbaocluster protections missing: https=%v unsafeURL=%v transitAuthorizer=%v backupAuthorizer=%v systemSecret=%v",
			foundHTTPS,
			foundUnsafeURLComponents,
			foundSecretAuthorizer,
			foundBackupSecretAuthorizer,
			foundSystemSecretBlock,
		)
	}
}

func TestKustomizeDefault_OpenBaoRestorePolicyProtectsSecretRefs(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		gvk := u.GroupVersionKind()
		return gvk.Group == testAdmissionRegistrationGroup &&
			gvk.Kind == testKindVAP &&
			strings.HasSuffix(u.GetName(), "openbao-validate-openbaorestore")
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one openbao-validate-openbaorestore policy, got %d", len(objs))
	}

	validations, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "validations")
	if err != nil || !found {
		t.Fatalf("read policy validations: found=%v err=%v", found, err)
	}

	var foundRestoreSecretAuthorizer bool
	var foundSystemSecretBlock bool
	for _, validation := range validations {
		validationMap, ok := validation.(map[string]any)
		if !ok {
			continue
		}
		message, _ := validationMap["message"].(string)
		expression, _ := validationMap["expression"].(string)
		switch {
		case strings.Contains(message, "Users configuring restore credentials") &&
			strings.Contains(expression, `authorizer.group("")`) &&
			strings.Contains(expression, `resource("secrets")`) &&
			strings.Contains(expression, `check("get")`) &&
			strings.Contains(expression, `object.spec.source.target.credentialsSecretRef`) &&
			strings.Contains(expression, `object.spec.tokenSecretRef`):
			foundRestoreSecretAuthorizer = true
		case strings.Contains(message, "system secrets") &&
			strings.Contains(expression, "object.spec.source.target.credentialsSecretRef") &&
			strings.Contains(expression, "object.spec.tokenSecretRef") &&
			strings.Contains(expression, "root-token"):
			foundSystemSecretBlock = true
		}
	}

	if !foundRestoreSecretAuthorizer || !foundSystemSecretBlock {
		t.Fatalf(
			"openbao-validate-openbaorestore protections missing: authorizer=%v systemSecret=%v",
			foundRestoreSecretAuthorizer,
			foundSystemSecretBlock,
		)
	}
}

func TestKustomizeDefault_OpenBaoClusterCRDRejectsUpgradeStrategySwitches(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		return u.GetAPIVersion() == "apiextensions.k8s.io/v1" &&
			u.GetKind() == "CustomResourceDefinition" &&
			u.GetName() == "openbaoclusters.openbao.org"
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one openbaoclusters CRD, got %d", len(objs))
	}

	versions, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "versions")
	if err != nil || !found {
		t.Fatalf("read CRD versions: found=%v err=%v", found, err)
	}

	const wantMessage = "spec.upgrade.strategy is immutable after creation; " +
		"switching between RollingUpdate and BlueGreen is not supported."
	for _, version := range versions {
		versionMap, ok := version.(map[string]any)
		if !ok {
			continue
		}
		name, _ := versionMap["name"].(string)
		if name != "v1alpha1" {
			continue
		}

		schemaMap, ok := versionMap["schema"].(map[string]any)
		if !ok {
			t.Fatalf("v1alpha1 version missing schema")
		}
		openAPIV3Schema, ok := schemaMap["openAPIV3Schema"].(map[string]any)
		if !ok {
			t.Fatalf("v1alpha1 version missing openAPIV3Schema")
		}
		properties, ok := openAPIV3Schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("CRD root schema missing properties")
		}
		specSchema, ok := properties["spec"].(map[string]any)
		if !ok {
			t.Fatalf("CRD root schema missing spec property")
		}
		validations, ok := specSchema["x-kubernetes-validations"].([]any)
		if !ok {
			t.Fatal("spec schema missing x-kubernetes-validations")
		}

		for _, validation := range validations {
			validationMap, ok := validation.(map[string]any)
			if !ok {
				continue
			}
			message, _ := validationMap["message"].(string)
			rule, _ := validationMap["rule"].(string)
			if message == wantMessage && strings.Contains(rule, "oldSelf.upgrade.strategy") {
				return
			}
		}

		t.Fatalf("v1alpha1 CRD schema is missing the upgrade strategy transition rule")
	}

	t.Fatal("v1alpha1 version not found in openbaoclusters CRD")
}

func TestKustomizeDefault_ControllerOpenBaoAudienceMatchesProjection(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		return u.GetAPIVersion() == "apps/v1" &&
			u.GetKind() == "Deployment" &&
			u.GetName() == testControllerSAName
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one controller deployment, got %d", len(objs))
	}

	controller := objs[0]
	envAudience := kustomizeEnvVarValue(t, controller, "OPENBAO_JWT_AUDIENCE")
	projectedAudience := kustomizeProjectedTokenAudience(t, controller, "openbao-token")

	if envAudience != projectedAudience {
		t.Fatalf("controller OPENBAO_JWT_AUDIENCE=%q, projected openbao-token audience=%q", envAudience, projectedAudience)
	}
}

func TestKustomizeDefault_ManagerMetricsResourcesExposeControllerAndProvisioner(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, nil)

	testCases := []struct {
		name           string
		deploymentName string
		serviceName    string
		component      string
	}{
		{
			name:           "controller",
			deploymentName: testControllerSAName,
			serviceName:    "openbao-operator-controller-metrics-service",
			component:      "controller",
		},
		{
			name:           "provisioner",
			deploymentName: "openbao-operator-provisioner",
			serviceName:    "openbao-operator-provisioner-metrics-service",
			component:      testComponentProvisioner,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deployment := mustFindObject(t, objs, "apps/v1", "Deployment", tc.deploymentName)
			assertManagerDeploymentMetricsPort(t, deployment)

			service := mustFindObject(t, objs, "v1", "Service", tc.serviceName)
			assertManagerMetricsService(t, service, tc.component)
		})
	}
}

func TestKustomizeDefault_ProvisionerRoleDoesNotReadServiceAccounts(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		return u.GetAPIVersion() == "rbac.authorization.k8s.io/v1" &&
			u.GetKind() == "ClusterRole" &&
			u.GetName() == "openbao-operator-provisioner-role"
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one provisioner ClusterRole, got %d", len(objs))
	}

	rules, found, err := unstructured.NestedSlice(objs[0].Object, "rules")
	if err != nil || !found {
		t.Fatalf("read provisioner rules: found=%v err=%v", found, err)
	}

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		resources, _ := ruleMap["resources"].([]any)
		for _, resource := range resources {
			if resource == "serviceaccounts" {
				t.Fatalf("provisioner ClusterRole unexpectedly grants serviceaccounts access: %#v", ruleMap)
			}
		}
	}
}

func TestKustomizeDefault_ControllerRoleReadsGatewayAPIByGetOnly(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "default"))
	objs := parseYAMLToUnstructured(t, yamlBytes, func(u *unstructured.Unstructured) bool {
		return u.GetAPIVersion() == "rbac.authorization.k8s.io/v1" &&
			u.GetKind() == "ClusterRole" &&
			u.GetName() == "openbao-operator-controller-openbaocluster-role"
	})

	if len(objs) != 1 {
		t.Fatalf("expected exactly one controller ClusterRole, got %d", len(objs))
	}

	rules, found, err := unstructured.NestedSlice(objs[0].Object, "rules")
	if err != nil || !found {
		t.Fatalf("read controller rules: found=%v err=%v", found, err)
	}

	foundGatewayRule := false
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}

		resources, _ := ruleMap["resources"].([]any)
		if len(resources) != 2 {
			continue
		}

		hasGateways := false
		hasGatewayClasses := false
		for _, resource := range resources {
			switch resource {
			case "gateways":
				hasGateways = true
			case "gatewayclasses":
				hasGatewayClasses = true
			}
		}
		if !hasGateways || !hasGatewayClasses {
			continue
		}

		verbs, _ := ruleMap["verbs"].([]any)
		if len(verbs) != 1 || verbs[0] != "get" {
			t.Fatalf("controller ClusterRole must grant Gateway API reads via get only, got %#v", ruleMap)
		}

		foundGatewayRule = true
	}

	if !foundGatewayRule {
		t.Fatal("expected controller ClusterRole to include get-only access to gateways and gatewayclasses")
	}
}

func TestKustomizeSingleTenantOverlay_BakesInNamespaceScopeAndRemovesProvisioner(t *testing.T) {
	yamlBytes := kustomizeBuild(t, filepath.Join("..", "..", "config", "overlays", "single-tenant"))
	objs := parseYAMLToUnstructured(t, yamlBytes, nil)

	var controller *unstructured.Unstructured
	var singleTenantBinding *unstructured.Unstructured
	var hasOperatorNamespace bool

	for _, obj := range objs {
		failIfProvisionerObject(t, obj)

		switch obj.GetKind() {
		case "Namespace":
			if obj.GetName() == testDefaultOperatorNS {
				hasOperatorNamespace = true
			}
		case "Deployment":
			if obj.GetName() == testControllerSAName {
				controller = obj
			}
		case "RoleBinding":
			if obj.GetName() == "openbao-operator-single-tenant" {
				singleTenantBinding = obj
			}
		}
	}

	if !hasOperatorNamespace {
		t.Fatal("single-tenant overlay did not include operator namespace")
	}
	if controller == nil {
		t.Fatal("single-tenant overlay missing controller deployment")
	}
	if singleTenantBinding == nil {
		t.Fatal("single-tenant overlay missing target namespace rolebinding")
	}
	if singleTenantBinding.GetNamespace() != testSingleTenantTargetNS {
		t.Fatalf(
			"single-tenant rolebinding namespace = %q, want %q",
			singleTenantBinding.GetNamespace(),
			testSingleTenantTargetNS,
		)
	}

	if got := kustomizeEnvVarValue(t, controller, "WATCH_NAMESPACE"); got != testSingleTenantTargetNS {
		t.Fatalf("WATCH_NAMESPACE = %q, want %q", got, testSingleTenantTargetNS)
	}

	subjects, found, err := unstructured.NestedSlice(singleTenantBinding.Object, "subjects")
	if err != nil || !found || len(subjects) != 1 {
		t.Fatalf("read rolebinding subjects: found=%v len=%d err=%v", found, len(subjects), err)
	}
	subject, ok := subjects[0].(map[string]any)
	if !ok {
		t.Fatalf("rolebinding subject has unexpected type %T", subjects[0])
	}
	if got, _ := subject["name"].(string); got != testControllerSAName {
		t.Fatalf("rolebinding subject name = %q, want %q", got, testControllerSAName)
	}
	if got, _ := subject["namespace"].(string); got != testDefaultOperatorNS {
		t.Fatalf("rolebinding subject namespace = %q, want %q", got, testDefaultOperatorNS)
	}
}

func failIfProvisionerObject(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()

	if obj.GetLabels()["app.kubernetes.io/component"] != testComponentProvisioner {
		return
	}
	t.Fatalf("unexpected provisioner %s in single-tenant overlay: %s", strings.ToLower(obj.GetKind()), obj.GetName())
}

func kustomizeEnvVarValue(t *testing.T, obj *unstructured.Unstructured, name string) string {
	t.Helper()

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		t.Fatalf("containers not found: %v", err)
	}

	for _, container := range containers {
		containerMap, ok := container.(map[string]any)
		if !ok || containerMap["name"] != "manager" {
			continue
		}
		envs, ok := containerMap["env"].([]any)
		if !ok {
			t.Fatalf("manager env not found")
		}
		for _, env := range envs {
			envMap, ok := env.(map[string]any)
			if !ok {
				continue
			}
			if envMap["name"] == name {
				value, ok := envMap["value"].(string)
				if !ok {
					t.Fatalf("env %s has no string value", name)
				}
				return value
			}
		}
	}

	t.Fatalf("env %s not found", name)
	return ""
}

func assertManagerDeploymentMetricsPort(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		t.Fatalf("deployment %s containers not found: found=%v err=%v", obj.GetName(), found, err)
	}

	manager, ok := containers[0].(map[string]any)
	if !ok {
		t.Fatalf("deployment %s container has unexpected type %T", obj.GetName(), containers[0])
	}
	args, _ := manager["args"].([]any)
	if !containsAny(args, "--metrics-bind-address=:8443") {
		t.Fatalf("deployment %s missing metrics bind-address arg: %#v", obj.GetName(), args)
	}

	ports, _ := manager["ports"].([]any)
	for _, port := range ports {
		portMap, ok := port.(map[string]any)
		if !ok {
			continue
		}
		if portMap["name"] == "https" && numericYAMLValue(portMap["containerPort"]) == 8443 {
			return
		}
	}
	t.Fatalf("deployment %s missing https container port 8443: %#v", obj.GetName(), ports)
}

func assertManagerMetricsService(t *testing.T, obj *unstructured.Unstructured, component string) {
	t.Helper()

	selector, found, err := unstructured.NestedStringMap(obj.Object, "spec", "selector")
	if err != nil || !found {
		t.Fatalf("service %s selector not found: found=%v err=%v", obj.GetName(), found, err)
	}
	if selector["app.kubernetes.io/component"] != component {
		t.Fatalf("service %s component selector = %#v, want %q", obj.GetName(), selector, component)
	}

	ports, found, err := unstructured.NestedSlice(obj.Object, "spec", "ports")
	if err != nil || !found || len(ports) != 1 {
		t.Fatalf("service %s ports = %#v found=%v err=%v", obj.GetName(), ports, found, err)
	}
	port, ok := ports[0].(map[string]any)
	if !ok {
		t.Fatalf("service %s port has unexpected type %T", obj.GetName(), ports[0])
	}
	if port["name"] != "https" ||
		numericYAMLValue(port["port"]) != 8443 ||
		numericYAMLValue(port["targetPort"]) != 8443 {
		t.Fatalf("service %s metrics port = %#v, want https:8443", obj.GetName(), port)
	}
}

func numericYAMLValue(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func containsAny(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func kustomizeProjectedTokenAudience(t *testing.T, obj *unstructured.Unstructured, volumeName string) string {
	t.Helper()

	volumes, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	if err != nil || !found {
		t.Fatalf("volumes not found: %v", err)
	}

	for _, volume := range volumes {
		volumeMap, ok := volume.(map[string]any)
		if !ok || volumeMap["name"] != volumeName {
			continue
		}
		projected, ok := volumeMap["projected"].(map[string]any)
		if !ok {
			t.Fatalf("volume %s is not projected", volumeName)
		}
		sources, ok := projected["sources"].([]any)
		if !ok || len(sources) == 0 {
			t.Fatalf("volume %s has no projected sources", volumeName)
		}
		first, ok := sources[0].(map[string]any)
		if !ok {
			t.Fatalf("volume %s source has unexpected type %T", volumeName, sources[0])
		}
		token, ok := first["serviceAccountToken"].(map[string]any)
		if !ok {
			t.Fatalf("volume %s first source is not a serviceAccountToken", volumeName)
		}
		audience, ok := token["audience"].(string)
		if !ok {
			t.Fatalf("volume %s serviceAccountToken.audience missing", volumeName)
		}
		return audience
	}

	t.Fatalf("volume %s not found", volumeName)
	return ""
}

func isClusterScopedManifestObject(gvk schema.GroupVersionKind) bool {
	if gvk.Group == testRBACGroup && (gvk.Kind == testKindClusterRole || gvk.Kind == testKindClusterRoleBinding) {
		return true
	}
	if gvk.Group == testAdmissionRegistrationGroup &&
		(gvk.Kind == testKindVAP || gvk.Kind == testKindVAPBinding) {
		return true
	}
	if gvk.Group == "apiextensions.k8s.io" && gvk.Kind == "CustomResourceDefinition" {
		return true
	}
	return false
}
