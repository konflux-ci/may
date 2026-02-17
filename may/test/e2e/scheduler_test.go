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

	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/test/utils"
)

// SchedulerContexts registers the Claim/Scheduler (Claim → Runner) test contexts.
// Runners live in the manager namespace (may-system); Claims/Pods live in a tenant namespace.
// Serial ensures It specs run one after another, not concurrently.
func SchedulerContexts() {
	Context("Claim / Scheduler (Claim → Runner)", Ordered, func() {
		BeforeAll(func() {
			By("creating tenant namespace for Scheduler tests")
			cmd := exec.Command("kubectl", "create", "namespace", schedulerTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", schedulerTestNamespace,
				"konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting Scheduler test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", schedulerTestNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("schedules Claim when a free Runner (matching flavor) exists", func() {
			runnerName := "runner-schedule-claim"
			podName := "pod-schedule-claim"
			applySpecificationWithStatus(runnerYAMLWithReadyStatus(runnerName, namespace, schedulerFlavor))
			defer deleteRunner(runnerName)
			defer deletePod(schedulerTestNamespace, podName)

			createPodWithFlavor(podName, schedulerTestNamespace, schedulerFlavor)

			By("verifying Claim gets Claimed and Runner gets spec.inUseBy set")
			Eventually(func(g Gomega) {
				c := getClaim(g, schedulerTestNamespace, podName)
				g.Expect(claim.IsClaimed(*c)).To(BeTrue(), "expected Claim to be Claimed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(r.Spec.InUseBy).NotTo(BeNil())
				g.Expect(r.Spec.InUseBy.Name).To(Equal(podName))
				g.Expect(r.Spec.InUseBy.Namespace).To(Equal(schedulerTestNamespace))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting Pod and verifying Claim is deleted")
			deletePod(schedulerTestNamespace, podName)
			waitForClaimDeleted(schedulerTestNamespace, podName)
		})

		It("keeps Claim Pending when no Runner exists for the flavor", func() {
			podName := "pod-pending-no-runner"
			defer deletePod(schedulerTestNamespace, podName)
			createPodWithFlavor(podName, schedulerTestNamespace, noRunnerFlavor)

			By("verifying Claim stays Pending")
			Consistently(func(g Gomega) {
				c := getClaim(g, schedulerTestNamespace, podName)
				g.Expect(claim.IsPending(*c)).To(BeTrue(), "expected Claim to stay Pending")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting Pod and verifying Claim is deleted")
			deletePod(schedulerTestNamespace, podName)
			waitForClaimDeleted(schedulerTestNamespace, podName)
		})

		It("schedules one Claim and keeps the other Pending with a single Runner; releasing first releases Runner", func() {
			runnerName := "runner-single-two-claims"
			pod1Name := "pod-single-runner-1"
			pod2Name := "pod-single-runner-2"
			applySpecificationWithStatus(runnerYAMLWithReadyStatus(runnerName, namespace, schedulerFlavor))
			defer deleteRunner(runnerName)
			defer deletePod(schedulerTestNamespace, pod1Name)
			defer deletePod(schedulerTestNamespace, pod2Name)

			createPodWithFlavor(pod1Name, schedulerTestNamespace, schedulerFlavor)
			createPodWithFlavor(pod2Name, schedulerTestNamespace, schedulerFlavor)

			By("verifying first Claim is Claimed and second is Pending")
			Eventually(func(g Gomega) {
				c1 := getClaim(g, schedulerTestNamespace, pod1Name)
				g.Expect(claim.IsClaimed(*c1)).To(BeTrue())
				c2 := getClaim(g, schedulerTestNamespace, pod2Name)
				g.Expect(claim.IsPending(*c2)).To(BeTrue())
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting the first Claim to release the Runner")
			cmd := exec.Command("kubectl", "delete", "claim", pod1Name, "-n", schedulerTestNamespace, "--ignore-not-found")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Runner is released (finalizer deletes the Runner)")
			Eventually(func(g Gomega) {
				_, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).To(BeKubectlNotFound(), "expected Runner to be deleted when Claim is deleted")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting Pods and verifying Claims are deleted")
			deletePod(schedulerTestNamespace, pod1Name)
			deletePod(schedulerTestNamespace, pod2Name)
			waitForClaimDeleted(schedulerTestNamespace, pod1Name)
			waitForClaimDeleted(schedulerTestNamespace, pod2Name)
		})

		It("releases the Runner when Claim is deleted (finalizer clears reservation or deletes Runner)", func() {
			runnerName := "runner-claim-deletion"
			podName := "pod-claim-deletion"
			applySpecificationWithStatus(runnerYAMLWithReadyStatus(runnerName, namespace, schedulerFlavor))
			defer deleteRunner(runnerName)
			defer deletePod(schedulerTestNamespace, podName)

			createPodWithFlavor(podName, schedulerTestNamespace, schedulerFlavor)

			By("waiting for Claim to be Claimed")
			Eventually(func(g Gomega) {
				c := getClaim(g, schedulerTestNamespace, podName)
				g.Expect(claim.IsClaimed(*c)).To(BeTrue())
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting the Claim")
			cmd := exec.Command("kubectl", "delete", "claim", podName, "-n", schedulerTestNamespace, "--ignore-not-found")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Runner is released (deleted by finalizer)")
			Eventually(func(g Gomega) {
				_, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).To(BeKubectlNotFound(), "expected Runner to be deleted when Claim is deleted")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting Pod and verifying Claim is deleted")
			deletePod(schedulerTestNamespace, podName)
			waitForClaimDeleted(schedulerTestNamespace, podName)
		})
	})
}
