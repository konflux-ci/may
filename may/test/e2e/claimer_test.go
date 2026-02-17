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
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/may/pkg/pod"
	"github.com/konflux-ci/may/test/utils"
)

// ClaimerContexts registers the Claimer (Pod → Claim) test contexts. Call this from
// within the Manager context so that Claimer tests run after the manager and webhooks are up.
func ClaimerContexts() {
	Context("Claimer", func() {
		BeforeAll(func() {
			By("creating tenant namespace for Claimer tests")
			cmd := exec.Command("kubectl", "create", "namespace", claimerTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", claimerTestNamespace,
				"konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting Claimer test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", claimerTestNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("creates a Claim for Pod with flavor annotation in tenant namespace", Label("smoke"), func() {
			podName := "pod-claimer-with-flavor"
			flavorAnnotation := pod.KueueFlavorLabelPrefix + "aws-linux-arm64"

			By("creating a Pod with flavor annotation in tenant namespace")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", claimerTestNamespace,
				"--overrides", fmt.Sprintf(`{"metadata":{"annotations":{"%s":"1"}}}`, flavorAnnotation),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying a Claim is created with same name/namespace, correct spec.for, spec.flavor, and ownerReference to Pod")
			Eventually(func(g Gomega) {
				c := getClaim(g, claimerTestNamespace, podName)
				g.Expect(c.Name).To(Equal(podName))
				g.Expect(c.Namespace).To(Equal(claimerTestNamespace))
				g.Expect(c.Spec.Flavor).To(Equal("aws-linux-arm64"))
				g.Expect(c.Spec.For.Kind).To(Equal("Pod"))
				g.Expect(c.Spec.For.Name).To(Equal(podName))
				g.Expect(c.OwnerReferences).NotTo(BeEmpty())
				g.Expect(c.OwnerReferences[0].Kind).To(Equal("Pod"))
				g.Expect(c.OwnerReferences[0].Name).To(Equal(podName))
			}).Should(Succeed())
		})

		It("does not create a Claim for Pod without flavor annotation in tenant namespace", func() {
			podName := "pod-claimer-without-flavor"

			By("creating a Pod without flavor annotation in tenant namespace")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", claimerTestNamespace,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying no Claim is created")
			Eventually(func(g Gomega) {
				_, err := getClaimOrErr(g, claimerTestNamespace, podName)
				g.Expect(err).To(BeKubectlNotFound(), "expected no Claim for Pod without flavor")
			}).Should(Succeed())
		})
	})

	Context("Claimer - non-tenant namespace", func() {
		const claimerNonTenantNamespace = "e2e-claimer-non-tenant"

		BeforeAll(func() {
			By("creating namespace without tenant label")
			cmd := exec.Command("kubectl", "create", "namespace", claimerNonTenantNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting non-tenant Claimer test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", claimerNonTenantNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("does not create a Claim for Pod with valid flavor annotation in non-tenant namespace", func() {
			podName := "pod-claimer-with-flavor-non-tenant"
			flavorAnnotation := pod.KueueFlavorLabelPrefix + "aws-linux-arm64"

			By("creating a Pod with valid flavor annotation in non-tenant namespace")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", claimerNonTenantNamespace,
				"--overrides", fmt.Sprintf(`{"metadata":{"annotations":{"%s":"1"}}}`, flavorAnnotation),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying no Claim is created")
			Eventually(func(g Gomega) {
				_, err := getClaimOrErr(g, claimerTestNamespace, podName)
				g.Expect(err).To(BeKubectlNotFound(), "Claimer must not create Claim in non-tenant namespace")
			}).Should(Succeed())
		})

		It("does not create a Claim for Pod without annotation in non-tenant namespace", func() {
			podName := "pod-claimer-without-annotation-non-tenant"

			By("creating a Pod without flavor annotation in non-tenant namespace")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", claimerNonTenantNamespace,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying no Claim is created")
			Eventually(func(g Gomega) {
				_, err := getClaimOrErr(g, claimerTestNamespace, podName)
				g.Expect(err).To(BeKubectlNotFound(), "expected no Claim for Pod without annotation in non-tenant namespace")
			}).Should(Succeed())
		})
	})
}
