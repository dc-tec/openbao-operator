//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

var _ = Describe("Claims Smoke", Label("claims", "claims-smoke", "critical"), Ordered, func() {
	ctx := context.Background()

	var (
		f                   *framework.Framework
		c                   client.Client
		catalog             sameClusterClaimCatalog
		claim               *openbaov1alpha1.OpenBaoClusterClaim
		materializedLocal   *openbaov1alpha1.NamespacedReference
		connectionSecretRef string
	)

	BeforeAll(func() {
		if !serviceClaimsE2EEnabled() {
			Skip("claim smoke suite requires E2E_ENABLE_SERVICE_CLAIMS=true")
		}

		var err error
		f, err = framework.NewSetup(ctx, "claims-smoke", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
		c = f.Client
		catalog = newSameClusterClaimCatalog(f.Namespace)

		Expect(createObjects(
			ctx,
			c,
			catalog.bootstrapAuthSecret(f.Namespace),
			catalog.backupProfile(),
			catalog.internalExposureClass(),
			catalog.secretBootstrapProfile(),
			catalog.serviceProfile(),
			catalog.serviceOffering(),
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

	It("binds a stable service offering to a ready same-cluster claim with secret-backed bootstrap", Label(
		"case:claims-smoke-offering-secret-bootstrap",
		"covers:service-offering-binding",
		"covers:secret-bootstrap-projection",
		"covers:same-cluster-materialization",
		"covers:same-cluster-connection",
	), func() {
		claim = catalog.sameClusterClaim(operatorNamespace, claimScopedName("claim", f.Namespace), f.TenantName)
		Expect(c.Create(ctx, claim)).To(Succeed())

		By("waiting for the claim to bind the selected offering to one immutable service profile")
		updated, err := waitForClaimPinnedBinding(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			catalog.OfferingName,
			catalog.ServiceProfileName,
			6*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the claim to reach Ready")
		updated, err = waitForClaimPhase(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			8*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Connection.Endpoint).NotTo(BeEmpty())
		Expect(updated.Status.Applied.ServiceOfferingRef).NotTo(BeNil())
		Expect(updated.Status.Applied.ServiceProfileRef).NotTo(BeNil())

		By("waiting for the same-cluster materialization to point at a concrete local OpenBaoCluster")
		materializedLocal, err = waitForClaimLocalClusterRef(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			3*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(materializedLocal.Namespace).To(Equal(f.Namespace))

		By("waiting for the local OpenBaoCluster to report Running")
		Expect(f.WaitForClusterPhase(
			ctx,
			materializedLocal.Name,
			openbaov1alpha1.ClusterPhaseRunning,
			8*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())

		By("waiting for the claim-owned connection Secret to be published")
		connectionSecret, err := waitForClaimConnectionSecret(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			5*time.Minute,
			framework.DefaultPollInterval,
		)
		Expect(err).NotTo(HaveOccurred())
		connectionSecretRef = connectionSecret.Name
		Expect(connectionSecret.Data).To(HaveKey("endpoint"))
		Expect(connectionSecret.Data).To(HaveKey("ca.crt"))
	})

	It("deletes the claim and cleans up local materialization artifacts", Label(
		"case:claims-smoke-cleanup",
		"covers:claim-deletion",
		"covers:same-cluster-cleanup",
	), func() {
		Expect(claim).NotTo(BeNil())
		Expect(materializedLocal).NotTo(BeNil())
		Expect(connectionSecretRef).NotTo(BeEmpty())

		By("deleting the claim")
		latest := &openbaov1alpha1.OpenBaoClusterClaim{}
		Expect(c.Get(ctx, client.ObjectKeyFromObject(claim), latest)).To(Succeed())
		Expect(c.Delete(ctx, latest)).To(Succeed())

		By("waiting for the claim to be removed")
		Expect(waitForClaimDeleted(
			ctx,
			c,
			claim.Namespace,
			claim.Name,
			4*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())

		By("waiting for the local OpenBaoCluster to be removed")
		Expect(waitForClusterDeleted(
			ctx,
			c,
			materializedLocal.Namespace,
			materializedLocal.Name,
			6*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())

		By("waiting for the claim-owned connection Secret to be removed")
		Expect(waitForSecretDeleted(
			ctx,
			c,
			claim.Namespace,
			connectionSecretRef,
			4*time.Minute,
			framework.DefaultPollInterval,
		)).To(Succeed())

		claim = nil
		materializedLocal = nil
		connectionSecretRef = ""
	})

})
