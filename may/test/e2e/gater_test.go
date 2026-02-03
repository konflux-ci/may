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
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/test/utils"
)

// GaterContexts registers the Gater (Claim Claimed → Pod ungated) test contexts.
// When a Claim becomes Claimed, the Gater removes the may scheduling gate from the Pod.
func GaterContexts() {
	Context("Gater (Claim Claimed → Pod ungated)", func() {
		BeforeAll(func() {
			By("creating tenant namespace for Gater tests")
			cmd := exec.Command("kubectl", "create", "namespace", gaterTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", gaterTestNamespace,
				"konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting Gater test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", gaterTestNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("removes Pod scheduling gate when Claim is Claimed", func() {
			runnerName := "runner-gater-claimed"
			podName := "pod-gater-claimed"
			applySpecificationWithStatus(runnerYAMLWithReadyStatus(runnerName, namespace, gaterFlavor))
			defer deleteRunner(runnerName)
			defer deletePod(gaterTestNamespace, podName)

			createPodWithFlavor(podName, gaterTestNamespace, gaterFlavor)

			By("verifying Claim gets Claimed")
			Eventually(func(g Gomega) {
				c := getClaim(g, gaterTestNamespace, podName)
				g.Expect(claim.IsClaimed(*c)).To(BeTrue(), "expected Claim to be Claimed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying Pod's scheduling gate is removed")
			Eventually(func(g Gomega) {
				p := getPod(g, gaterTestNamespace, podName)
				g.Expect(slices.ContainsFunc(p.Spec.SchedulingGates, func(s corev1.PodSchedulingGate) bool {
					return s.Name == constants.MayPodSchedulingGate
				})).To(BeFalse(), "expected may scheduling gate to be removed when Claim is Claimed")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("keeps Pod scheduling gate while Claim is Pending", func() {
			podName := "pod-gater-pending"
			defer deletePod(gaterTestNamespace, podName)
			createPodWithFlavor(podName, gaterTestNamespace, gaterNoRunnerFlavor)

			By("verifying Claim stays Pending (no Runner for flavor)")
			Eventually(func(g Gomega) {
				c := getClaim(g, gaterTestNamespace, podName)
				g.Expect(claim.IsPending(*c)).To(BeTrue(), "expected Claim to be Pending")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying Pod still has the may scheduling gate")
			Consistently(func(g Gomega) {
				p := getPod(g, gaterTestNamespace, podName)
				g.Expect(p.Spec.SchedulingGates).NotTo(BeEmpty(), "expected at least one scheduling gate")
				g.Expect(slices.ContainsFunc(p.Spec.SchedulingGates, func(s corev1.PodSchedulingGate) bool {
					return s.Name == constants.MayPodSchedulingGate
				})).To(BeTrue(), "expected may scheduling gate to remain while Claim is Pending")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting Pod and verifying Claim is deleted")
			deletePod(gaterTestNamespace, podName)
			waitForClaimDeleted(gaterTestNamespace, podName)
		})
	})
}
