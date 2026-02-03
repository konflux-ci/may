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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/pod"
	"github.com/konflux-ci/may/test/utils"
)

// PodWebhookContexts registers the Pod webhook test contexts. Call this from
// within the Webhooks context in the main Manager Describe so that Pod webhook
// tests run after the manager and webhooks are up.
func PodWebhookContexts() {
	Context("Pod Webhook", func() {
		BeforeAll(func() {
			By("creating tenant namespace for Pod webhook tests")
			cmd := exec.Command("kubectl", "create", "namespace", webhookTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", webhookTestNamespace,
				"konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")
		})

		AfterAll(func() {
			By("deleting Pod webhook test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", webhookTestNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("adds scheduling gate to Pod with flavor annotation", func() {
			podName := "pod-with-flavor"
			flavorAnnotation := pod.KueueFlavorLabelPrefix + "aws-linux-arm64"

			By("creating a Pod with flavor annotation")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", webhookTestNamespace,
				"--overrides", fmt.Sprintf(`{"metadata":{"annotations":{"%s":"1"}}}`, flavorAnnotation),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Pod has the may scheduling gate with the correct name")
			Eventually(func(g Gomega) {
				p := getPod(g, webhookTestNamespace, podName)
				g.Expect(p.Spec.SchedulingGates).NotTo(BeEmpty(), "expected at least one scheduling gate")
				g.Expect(slices.ContainsFunc(p.Spec.SchedulingGates, func(s corev1.PodSchedulingGate) bool {
					return s.Name == constants.MayPodSchedulingGate
				})).To(BeTrue(), "expected scheduling gate %q to be present", constants.MayPodSchedulingGate)
			}).Should(Succeed())
		})

		It("leaves Pod without flavor annotation unchanged", func() {
			podName := "pod-without-flavor"

			By("creating a Pod without flavor annotation")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", webhookTestNamespace,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Pod does not have the may scheduling gate")
			Eventually(func(g Gomega) {
				p := getPod(g, webhookTestNamespace, podName)
				g.Expect(slices.ContainsFunc(p.Spec.SchedulingGates, func(s corev1.PodSchedulingGate) bool {
					return s.Name == constants.MayPodSchedulingGate
				})).To(BeFalse(), "Pod without flavor annotation should not have the may scheduling gate '%s'", constants.MayPodSchedulingGate)
			}).Should(Succeed())
		})
	})

	Context("Pod Webhook - non-tenant namespace", func() {
		const nonTenantNamespace = "e2e-pod-webhook-non-tenant"

		BeforeAll(func() {
			By("creating namespace without tenant label")
			cmd := exec.Command("kubectl", "create", "namespace", nonTenantNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting non-tenant Pod webhook test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", nonTenantNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("does not add scheduling gate to Pod with valid flavor annotation in non-tenant namespace", func() {
			podName := "pod-with-flavor-non-tenant"
			flavorAnnotation := pod.KueueFlavorLabelPrefix + "aws-linux-arm64"

			By("creating a Pod with valid flavor annotation in non-tenant namespace")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", nonTenantNamespace,
				"--overrides", fmt.Sprintf(`{"metadata":{"annotations":{"%s":"1"}}}`, flavorAnnotation),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying no scheduling gate is found on the Pod")
			Eventually(func(g Gomega) {
				p := getPod(g, nonTenantNamespace, podName)
				g.Expect(p.Spec.SchedulingGates).To(BeEmpty(),
					"Pod with valid annotation in non-tenant namespace must not be gated; expected no scheduling gates, got %v", p.Spec.SchedulingGates)
			}).Should(Succeed())
		})

		It("does not add scheduling gate to Pod without annotation in non-tenant namespace", func() {
			podName := "pod-without-annotation-non-tenant"

			By("creating a Pod without flavor annotation in non-tenant namespace")
			cmd := exec.Command("kubectl", "run", podName,
				"--image=registry.k8s.io/pause:3.9",
				"--restart=Never",
				"-n", nonTenantNamespace,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying no scheduling gate is added by the webhook")
			Eventually(func(g Gomega) {
				p := getPod(g, nonTenantNamespace, podName)
				g.Expect(p.Spec.SchedulingGates).To(BeEmpty(),
					"Pod without annotation in non-tenant namespace must not be gated; expected no scheduling gates, got %v", p.Spec.SchedulingGates)
			}).Should(Succeed())
		})
	})
}
