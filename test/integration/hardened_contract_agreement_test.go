//go:build integration
// +build integration

package integration

import (
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
	hardenedfixtures "github.com/dc-tec/openbao-operator/test/fixtures/hardenedcontract"
)

func TestVAP_OpenBaoCluster_HardenedContractCatalog(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	const restrictedUsername = "hardened-contract-catalog-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, restrictedUsername)
	restrictedClient := newImpersonatedClient(t, restrictedUsername)

	for index, fixture := range hardenedfixtures.Fixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			cluster := hardenedfixtures.NewValidCluster(
				namespace,
				fmt.Sprintf("hardened-contract-%02d", index),
			)
			if fixture.Configure != nil {
				fixture.Configure(cluster)
			}

			requestClient := k8sClient
			if fixture.AuthorizationOnly {
				requestClient = restrictedClient
			}
			err := requestClient.Create(ctx, cluster, client.DryRunAll)

			if fixture.AdmissionRule == "" {
				if err != nil {
					t.Fatalf("expected admission to accept fixture, got: %v", err)
				}
				return
			}

			requireAdmissionDenied(t, err)
			rule, found := hardenedcontract.RuleFor(fixture.AdmissionRule)
			if !found {
				t.Fatalf("fixture references unknown admission rule %q", fixture.AdmissionRule)
			}
			if !strings.Contains(err.Error(), rule.AdmissionMessage) {
				t.Fatalf(
					"admission error for rule %q does not contain catalog message %q: %v",
					rule.ID,
					rule.AdmissionMessage,
					err,
				)
			}
		})
	}
}
