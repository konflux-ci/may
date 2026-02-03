//go:build e2e
// +build e2e

/*
Copyright 2026.

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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/test/utils"
)

// DynamicHostAutoscalerContexts registers the DynamicHostAutoscaler (provisioner) test contexts.
// The provisioner watches Pending Claims and creates a DynamicHost per Claim when a DynamicHostAutoscaler
// matches the Claim's flavor. Tests use a tenant namespace so Claimer creates Claims from Pods.
func DynamicHostAutoscalerContexts() {
	Context("DynamicHostAutoscaler", Ordered, func() {
		BeforeAll(func() {
			By("creating namespace for DynamicHostAutoscaler tests")
			cmd := exec.Command("kubectl", "create", "namespace", autoscalerTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			By("labeling the namespace as tenant (for Claimer to create Claims from Pods)")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", autoscalerTestNamespace,
				constants.TenantNamespaceLabelKey+"="+constants.TenantNamespaceLabelValue)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting DynamicHosts created by provisioner (names = Pod/Claim names)")
			for _, name := range []string{"pod-no-autoscaler", "pod-autoscaler-claim", "pod-autoscaler-1", "pod-autoscaler-2"} {
				deleteDynamicHost(namespace, name)
			}
			By("deleting DynamicHostAutoscaler")
			deleteDynamicHostAutoscaler(namespace, autoscalerName)
			By("deleting DynamicHostAutoscaler test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", autoscalerTestNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("creates DynamicHost when Claim is Pending and Autoscaler matches flavor", func() {
			By("creating DynamicHostAutoscaler with matching flavor")
			applySpecification(dynamicHostAutoscalerYAML(autoscalerName, namespace, autoscalerFlavor, autoscalerFlavor, autoscalerRootKeyName))

			podName := "pod-autoscaler-claim"
			createPodWithFlavor(podName, autoscalerTestNamespace, autoscalerFlavor)
			DeferCleanup(func() { deletePod(autoscalerTestNamespace, podName) })

			By("waiting for Claim to be created (Pending)")
			Eventually(func(g Gomega) {
				c := getClaim(g, autoscalerTestNamespace, podName)
				g.Expect(c.Name).To(Equal(podName))
				g.Expect(claim.IsPending(*c)).To(BeTrue(), "Claim should be Pending until scheduled")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for provisioner to create DynamicHost named like the Claim with spec from Autoscaler template")
			Eventually(func(g Gomega) {
				h := getDynamicHost(g, namespace, podName)
				g.Expect(h.Name).To(Equal(podName))
				g.Expect(h.Namespace).To(Equal(namespace))
				g.Expect(h.Spec.Flavor).To(Equal(autoscalerFlavor))
				g.Expect(h.Spec.RootKey.Name).To(Equal(podName), "provisioner overwrites rootKey.name with host name")
				g.Expect(h.Spec.Runner.Resources).To(HaveKey(corev1.ResourceName(autoscalerFlavor)))
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("does not create DynamicHost when no Autoscaler matches Claim flavor", func() {
			podName := "pod-no-autoscaler"
			createPodWithFlavor(podName, autoscalerTestNamespace, autoscalerNoMatchFlavor)
			DeferCleanup(func() { deletePod(autoscalerTestNamespace, podName) })

			By("waiting for Claim to be created and stay Pending (no Runner, no matching Autoscaler)")
			Eventually(func(g Gomega) {
				c := getClaim(g, autoscalerTestNamespace, podName)
				g.Expect(claim.IsPending(*c)).To(BeTrue(), "Claim should be Pending")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying no DynamicHost is created for the Claim (no Autoscaler matches flavor)")
			Consistently(func(g Gomega) {
				_, err := getDynamicHostOrNotFound(g, namespace, podName)
				g.Expect(err).To(HaveOccurred(), "expected no DynamicHost when no Autoscaler matches Claim flavor")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("creates one DynamicHost per Pending Claim when multiple Claims match the same Autoscaler", func() {
			pod1Name := "pod-autoscaler-1"
			pod2Name := "pod-autoscaler-2"
			createPodWithFlavor(pod1Name, autoscalerTestNamespace, autoscalerFlavor)
			createPodWithFlavor(pod2Name, autoscalerTestNamespace, autoscalerFlavor)
			DeferCleanup(func() {
				deletePod(autoscalerTestNamespace, pod1Name)
				deletePod(autoscalerTestNamespace, pod2Name)
			})

			By("waiting for both Claims to be Pending")
			Eventually(func(g Gomega) {
				c1 := getClaim(g, autoscalerTestNamespace, pod1Name)
				c2 := getClaim(g, autoscalerTestNamespace, pod2Name)
				g.Expect(claim.IsPending(*c1)).To(BeTrue())
				g.Expect(claim.IsPending(*c2)).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for provisioner to create one DynamicHost per Claim (by CreationTimestamp order)")
			Eventually(func(g Gomega) {
				h1 := getDynamicHost(g, namespace, pod1Name)
				h2 := getDynamicHost(g, namespace, pod2Name)
				g.Expect(h1.Spec.Flavor).To(Equal(autoscalerFlavor))
				g.Expect(h2.Spec.Flavor).To(Equal(autoscalerFlavor))
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
}
