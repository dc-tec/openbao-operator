//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/test/e2e/framework"
	e2ehelpers "github.com/openbao/operator/test/e2e/helpers"
)

func sentinelDebounceJitterSeconds(clusterName string, jitterRangeSeconds uint64) uint64 {
	if clusterName == "" || jitterRangeSeconds == 0 {
		return 0
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(clusterName))
	return h.Sum64() % jitterRangeSeconds
}

func pickClusterNamesForJitters(prefix string, jitterRangeSeconds uint64, desiredJitters []uint64) ([]string, map[string]uint64, error) {
	if prefix == "" {
		return nil, nil, fmt.Errorf("prefix is required")
	}
	if jitterRangeSeconds == 0 {
		return nil, nil, fmt.Errorf("jitter range seconds must be > 0")
	}
	if len(desiredJitters) == 0 {
		return nil, nil, fmt.Errorf("desired jitters must be non-empty")
	}

	names := make([]string, 0, len(desiredJitters))
	jitters := make(map[string]uint64, len(desiredJitters))
	used := make(map[string]struct{}, len(desiredJitters))

	const maxAttempts = 500
	for _, desired := range desiredJitters {
		found := false
		for i := 0; i < maxAttempts; i++ {
			candidate := fmt.Sprintf("%s-%d", prefix, i)
			if _, ok := used[candidate]; ok {
				continue
			}

			j := sentinelDebounceJitterSeconds(candidate, jitterRangeSeconds)
			if j != desired {
				continue
			}

			used[candidate] = struct{}{}
			names = append(names, candidate)
			jitters[candidate] = j
			found = true
			break
		}
		if !found {
			return nil, nil, fmt.Errorf("failed to find cluster name for desired jitter %d within %d attempts", desired, maxAttempts)
		}
	}

	return names, jitters, nil
}

var _ = Describe("Scale: Sentinel jitter and noise control", Label("sentinel", "scale", "slow"), Ordered, func() {
	ctx := context.Background()

	var (
		cfg    *rest.Config
		scheme *runtime.Scheme
		c      client.Client
		f      *framework.Framework
	)

	BeforeAll(func() {
		var err error

		cfg, err = ctrlconfig.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(openbaov1alpha1.AddToScheme(scheme)).To(Succeed())

		c, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		f, err = framework.New(ctx, c, "scale-noise", operatorNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if f == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		_ = f.Cleanup(cleanupCtx)
	})

	It("does not synchronize drift triggers across many clusters", func() {
		// Default is 5s unless the operator is configured otherwise.
		const jitterRangeSeconds = uint64(5)
		clusterNames, clusterJitters, err := pickClusterNamesForJitters("scale-sentinel", jitterRangeSeconds, []uint64{0, 2, 4})
		Expect(err).NotTo(HaveOccurred())

		By("creating multiple OpenBaoClusters with Sentinel enabled in the same namespace")
		for _, clusterName := range clusterNames {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: f.Namespace,
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Profile:  openbaov1alpha1.ProfileDevelopment,
					Version:  openBaoVersion,
					Image:    openBaoImage,
					Replicas: 1,
					InitContainer: &openbaov1alpha1.InitContainerConfig{
						Enabled: true,
						Image:   configInitImage,
					},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled:  true,
						Requests: framework.DefaultAdminSelfInitRequests(),
					},
					TLS: openbaov1alpha1.TLSConfig{
						Enabled:        true,
						Mode:           openbaov1alpha1.TLSModeOperatorManaged,
						RotationPeriod: "720h",
					},
					Storage: openbaov1alpha1.StorageConfig{
						Size: "1Gi",
					},
					Network: &openbaov1alpha1.NetworkConfig{
						APIServerCIDR: kindDefaultServiceCIDR,
					},
					DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
					Sentinel: &openbaov1alpha1.SentinelConfig{
						Enabled: true,
					},
				},
			}

			Expect(c.Create(ctx, cluster)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter, "Created OpenBaoCluster %q (expected jitter=%ds)\n", clusterName, clusterJitters[clusterName])
		}

		By("waiting for Sentinel Deployments to become ready")
		for _, clusterName := range clusterNames {
			deploymentName := clusterName + constants.SentinelDeploymentNameSuffix
			Eventually(func(g Gomega) {
				deployment := &appsv1.Deployment{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: f.Namespace}, deployment)).To(Succeed())
				g.Expect(deployment.Status.ReadyReplicas).To(Equal(int32(1)))

				foundJitterEnv := false
				for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
					if env.Name == constants.EnvSentinelDebounceJitterSeconds {
						g.Expect(env.Value).To(Equal("5"))
						foundJitterEnv = true
						break
					}
				}
				g.Expect(foundJitterEnv).To(BeTrue(), "expected SENTINEL_DEBOUNCE_JITTER_SECONDS to be wired into the Sentinel Deployment")
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		}

		By("waiting for each cluster headless Service to exist")
		for _, clusterName := range clusterNames {
			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, svc)).To(Succeed())
			}, framework.DefaultWaitTimeout, framework.DefaultPollInterval).Should(Succeed())
		}

		By("introducing drift simultaneously by mutating a managed Service for each cluster")
		driftKey := "example.com/sentinel-drift"
		patchTimes := make(map[string]time.Time, len(clusterNames))

		for _, clusterName := range clusterNames {
			patchTime := time.Now()
			patchTimes[clusterName] = patchTime

			// ValidatingAdmissionPolicy blocks direct mutations of OpenBao-managed
			// resources unless "maintenance mode" is explicitly enabled on the object.
			// Use Server-Side Apply so managedFields reflect a non-operator manager,
			// allowing Sentinel to reliably classify this as drift.
			svcPatch := &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Service",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: f.Namespace,
					Annotations: map[string]string{
						constants.AnnotationMaintenance: "true",
						driftKey:                        fmt.Sprintf("drift-%d", patchTime.UnixNano()),
					},
				},
			}

			err := e2ehelpers.RunWithImpersonation(ctx, cfg, scheme, "e2e-break-glass", []string{"system:masters"}, func(ic client.Client) error {
				return ic.Patch(ctx, svcPatch, client.Apply, client.FieldOwner("e2e-break-glass"))
			})
			Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "Patched Service %q to introduce drift (at=%s)\n", clusterName, patchTime.Format(time.RFC3339Nano))
		}

		By("waiting for drift to be detected and corrected for each cluster")
		driftDetectedTimes := make(map[string]time.Time, len(clusterNames))
		for _, clusterName := range clusterNames {
			var detected time.Time
			Eventually(func(g Gomega) {
				cluster := &openbaov1alpha1.OpenBaoCluster{}
				g.Expect(c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: f.Namespace}, cluster)).To(Succeed())
				g.Expect(cluster.Status.Drift).NotTo(BeNil(), "expected drift status to be set")
				g.Expect(cluster.Status.Drift.LastDriftDetected).NotTo(BeNil(), "expected LastDriftDetected to be set")
				g.Expect(cluster.Status.Drift.DriftCorrectionCount).To(BeNumerically(">=", 1), "expected DriftCorrectionCount >= 1")
				g.Expect(cluster.Status.Drift.LastCorrectionTime).NotTo(BeNil(), "expected LastCorrectionTime to be set")

				detected = cluster.Status.Drift.LastDriftDetected.Time
				g.Expect(detected).To(BeTemporally(">=", patchTimes[clusterName]))
			}, framework.DefaultLongWaitTimeout, framework.DefaultPollInterval).Should(Succeed())

			driftDetectedTimes[clusterName] = detected
			_, _ = fmt.Fprintf(GinkgoWriter, "Cluster %q drift corrected at=%s\n", clusterName, detected.Format(time.RFC3339Nano))
		}

		By("verifying drift corrections are not fully synchronized")
		var (
			minLatency time.Duration
			maxLatency time.Duration
			hasLatency bool
		)
		for _, clusterName := range clusterNames {
			latency := driftDetectedTimes[clusterName].Sub(patchTimes[clusterName])
			if !hasLatency {
				minLatency = latency
				maxLatency = latency
				hasLatency = true
				continue
			}
			if latency < minLatency {
				minLatency = latency
			}
			if latency > maxLatency {
				maxLatency = latency
			}
		}
		Expect(hasLatency).To(BeTrue())

		// With jitter values {0,2,4} and a 2s base window, a healthy system should show a visible
		// spread in correction latency. Allow slack for controller scheduling and API latency.
		Expect(maxLatency - minLatency).To(BeNumerically(">=", 1*time.Second))
	})
})
