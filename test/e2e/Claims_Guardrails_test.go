//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Claims Guardrails", Label("claims", "claims-guardrails", "security", "admission"), Ordered, func() {
	ctx := context.Background()

	var (
		f                 *framework.Framework
		c                 client.Client
		catalog           sameClusterClaimCatalog
		claim             *openbaov1alpha1.OpenBaoClusterClaim
		materializedLocal *openbaov1alpha1.NamespacedReference
	)

	BeforeAll(func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim guardrail suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		var err error
		f, err = framework.NewSetup(ctx, "claims-guardrails", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c = f.Client
		catalog = newSameClusterClaimCatalog(f.Namespace)

		Expect(createObjects(
			ctx,
			c,
			catalog.backupProfile(),
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			catalog.serviceProfile(),
			catalog.serviceOffering(),
			catalog.bootstrapAuthSecret(f.Namespace),
		)).To(Succeed())
	})

	AfterAll(func() {
		if f == nil {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		if claim != nil {
			latest := &openbaov1alpha1.OpenBaoClusterClaim{}
			if err := c.Get(cleanupCtx, client.ObjectKeyFromObject(claim), latest); err == nil {
				_ = c.Delete(cleanupCtx, latest)
				_ = waitForClaimDeleted(cleanupCtx, c, latest.Namespace, latest.Name, 2*time.Minute, framework.DefaultPollInterval)
			}
		}

		_ = deleteObjects(
			cleanupCtx,
			c,
			catalog.serviceOffering(),
			catalog.serviceProfile(),
			catalog.secretBootstrapProfile(),
			catalog.internalExposureClass(),
			catalog.backupProfile(),
		)
		_ = f.Cleanup(cleanupCtx)
	})

	It("materializes a same-cluster claim before guardrail checks", Label(
		"case:claims-guardrails-materialize",
		"covers:claim-materialization",
	), func() {
		claim = catalog.sameClusterClaim(operatorNamespace, claimScopedName("guarded-claim", f.Namespace), f.TenantName)
		Expect(c.Create(ctx, claim)).To(Succeed())

		_, err := waitForClaimPhase(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())

		materializedLocal, err = waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			2*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("denies materialized claim spec mutation", Label(
		"case:claims-guardrails-offering-pin",
		"covers:claim-offering-pin",
	), func() {
		updated := &openbaov1alpha1.OpenBaoClusterClaim{}
		Expect(c.Get(ctx, client.ObjectKeyFromObject(claim), updated)).To(Succeed())
		original := updated.DeepCopy()
		updated.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: claimScopedName("other-profile", f.Namespace)}

		err := c.Patch(ctx, updated, client.MergeFrom(original))
		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "pinned by spec.serviceOfferingRef")).To(BeTrue(), "unexpected admission error: %v", err)
	})

	It("denies materialized claim spec mutation", Label(
		"case:claims-guardrails-spec-lock",
		"covers:claim-spec-lock",
	), func() {
		updated := &openbaov1alpha1.OpenBaoClusterClaim{}
		Expect(c.Get(ctx, client.ObjectKeyFromObject(claim), updated)).To(Succeed())
		original := updated.DeepCopy()
		updated.Spec.TenantRef = openbaov1alpha1.LocalReference{Name: claimScopedName("other-tenant", f.Namespace)}

		err := c.Patch(ctx, updated, client.MergeFrom(original))
		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "immutable after materialization")).To(BeTrue(), "unexpected admission error: %v", err)
	})

	It("denies direct deletion of a claim-managed local OpenBaoCluster", Label(
		"case:claims-guardrails-child-delete",
		"covers:claim-managed-child-protection",
	), func() {
		Expect(materializedLocal).NotTo(BeNil())

		localCluster := &openbaov1alpha1.OpenBaoCluster{}
		Expect(c.Get(ctx, client.ObjectKey{Namespace: materializedLocal.Namespace, Name: materializedLocal.Name}, localCluster)).To(Succeed())

		err := c.Delete(ctx, localCluster)
		Expect(err).To(HaveOccurred())
		Expect(strings.Contains(err.Error(), "Only the controller may delete claim-managed OpenBaoCluster resources.")).To(BeTrue(), "unexpected admission error: %v", err)
	})
})
