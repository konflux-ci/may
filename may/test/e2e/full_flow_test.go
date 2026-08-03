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
	"encoding/base64"
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

// FullFlowContexts registers the Full Flow (Pod → Claim → Schedule → Run) test contexts.
// A single test covers: happy path (Claim created → Claimed → gate removed → Pod scheduled)
// and Pod completion (Pod Succeeded → Claim deleted → Runner released).
func FullFlowContexts() {
	Context("Full Flow (Pod → Claim → Schedule → Run)", Ordered, func() {
		BeforeAll(func() {
			By("creating tenant namespace for Full Flow tests")
			cmd := exec.Command("kubectl", "create", "namespace", fullFlowTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", fullFlowTestNamespace,
				"konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting Full Flow test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", fullFlowTestNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("happy path: one Runner exists, create Pod with flavor → Claim created → Claimed → gate removed → Pod scheduled; Pod completion: Pod Succeeded → Claim deleted → Runner released", Label("smoke"), func() {
			runnerName := "runner-full-flow"
			podName := "pod-full-flow"
			expectedKey := "dummy-key-for-e2e"
			applySpecificationWithStatus(runnerYAMLWithReadyStatusForBinder(runnerName, namespace, fullFlowFlavor, fullFlowRunnerUser))
			defer deleteRunner(runnerName)
			defer deletePod(fullFlowTestNamespace, podName)
			defer deleteSecret(namespace, runnerName)

			By("creating Runner's Secret with id_rsa (required by RunnerBinder for OTP)")
			applySpecification(runnerSecretYAML(runnerName, namespace, base64.StdEncoding.EncodeToString([]byte(expectedKey))))

			By("creating OTP retriever Pod with flavor (goes through full flow, then retrieves key from OTP server)")
			applySpecification(otpRetrieverPodYAML(podName, podName, fullFlowTestNamespace, expectedKey, fullFlowFlavor))

			By("happy path: verifying Claim is created and Claimed")
			Eventually(func(g Gomega) {
				c := getClaim(g, fullFlowTestNamespace, podName)
				g.Expect(claim.IsClaimed(*c)).To(BeTrue(), "expected Claim to be Claimed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("happy path: verifying scheduling gate is removed and Pod is scheduled")
			Eventually(func(g Gomega) {
				p := getPod(g, fullFlowTestNamespace, podName)
				g.Expect(slices.ContainsFunc(p.Spec.SchedulingGates, func(s corev1.PodSchedulingGate) bool {
					return s.Name == constants.MayPodSchedulingGate
				})).To(BeFalse(), "expected may scheduling gate to be removed")
				g.Expect(p.Spec.NodeName).NotTo(BeEmpty(), "expected Pod to be scheduled (nodeName set)")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("Pod completion: waiting for Pod to reach Succeeded")
			Eventually(func(g Gomega) {
				p := getPod(g, fullFlowTestNamespace, podName)
				g.Expect(p.Status.Phase).To(Equal(corev1.PodSucceeded), "expected Pod to succeed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("Pod completion: verifying Claim is deleted and Runner is released")
			Eventually(func(g Gomega) {
				_, err := getClaimOrErr(g, fullFlowTestNamespace, podName)
				g.Expect(err).To(BeKubectlNotFound(), "expected Claim to be deleted after Pod completion")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				_, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).To(BeKubectlNotFound(), "expected Runner to be released (deleted or InUseBy cleared)")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
}
