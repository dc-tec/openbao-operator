package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	syaml "sigs.k8s.io/yaml"
)

func TestHelmControllerTokenIsolation(t *testing.T) {
	for _, test := range []struct {
		name, fullname string
		args           []string
	}{
		{name: "default", fullname: "test-openbao-operator"},
		{name: "custom", fullname: "custom", args: []string{"--set", "fullnameOverride=custom"}},
		{name: "single", fullname: "custom", args: []string{
			"--set", "fullnameOverride=custom", "--set", "tenancy.mode=single", "--set", "tenancy.targetNamespace=tenant",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rendered := renderChart(t, test.args...)
			var roles, bindings, deployments int
			for _, doc := range bytes.Split(rendered, []byte("\n---")) {
				var header struct {
					Kind     string
					Metadata struct{ Name string }
				}
				require.NoError(t, syaml.Unmarshal(doc, &header))
				if header.Metadata.Name == test.fullname+"-controller-token" {
					switch header.Kind {
					case "Role":
						var role rbacv1.Role
						require.NoError(t, syaml.Unmarshal(doc, &role))
						require.Equal(t, "openbao", role.Namespace)
						require.Equal(t, []rbacv1.PolicyRule{{
							APIGroups: []string{""}, Resources: []string{"serviceaccounts/token"},
							ResourceNames: []string{test.fullname + "-controller"}, Verbs: []string{"create"},
						}}, role.Rules)
						roles++
					case "RoleBinding":
						var binding rbacv1.RoleBinding
						require.NoError(t, syaml.Unmarshal(doc, &binding))
						require.Equal(t, "openbao", binding.Namespace)
						require.Equal(t, rbacv1.RoleRef{
							APIGroup: rbacv1.GroupName, Kind: "Role", Name: test.fullname + "-controller-token",
						}, binding.RoleRef)
						require.Equal(t, []rbacv1.Subject{{
							Kind: "ServiceAccount", Name: test.fullname + "-controller", Namespace: "openbao",
						}}, binding.Subjects)
						bindings++
					}
				}
				if header.Kind == "Deployment" && header.Metadata.Name == test.fullname+"-controller" {
					var deployment appsv1.Deployment
					require.NoError(t, syaml.Unmarshal(doc, &deployment))
					fields := map[string]string{}
					for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
						if env.ValueFrom != nil && env.ValueFrom.FieldRef != nil {
							fields[env.Name] = env.ValueFrom.FieldRef.FieldPath
						}
					}
					require.Equal(t, "metadata.name", fields["POD_NAME"])
					require.Equal(t, "metadata.uid", fields["POD_UID"])
					require.Equal(t, "metadata.namespace", fields["POD_NAMESPACE"])
					deployments++
				}
			}
			require.Equal(t, 1, roles)
			require.Equal(t, 1, bindings)
			require.Equal(t, 1, deployments)
		})
	}
}
