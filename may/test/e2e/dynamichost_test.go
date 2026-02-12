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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mayv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/test/utils"
)

// DynamicHostContexts registers the DynamicHost (in may) test contexts.
// Ordered tests share one DynamicHost; structure mirrors StaticHostContexts (one Runner per host, same name as host).
func DynamicHostContexts() {
	Context("DynamicHost (in may)", Ordered, func() {
		var dynamicHostName string = "dynamichost-full-workflow"

		BeforeAll(func() {
			By("creating namespace for DynamicHost tests")
			cmd := exec.Command("kubectl", "create", "namespace", dynamichostTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			By("labeling the namespace as tenant (for Claimer to create Claims from Pods)")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", dynamichostTestNamespace,
				constants.TenantNamespaceLabelKey+"="+constants.TenantNamespaceLabelValue)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("removing e2e finalizer from DynamicHost so it can be deleted")
			removeDynamicHostE2EFinalizer(Default, namespace, dynamicHostName)
			By("deleting DynamicHost and its Runner")
			deleteDynamicHost(namespace, dynamicHostName)
			By("deleting DynamicHost test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", dynamichostTestNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("does not create Runner when DynamicHost status.state is unset", func() {
			applySpecification(dynamicHostYAML(dynamicHostName, namespace, dynamichostFlavor, dynamichostFlavor, dynamichostRootKeyName, ""))
			By("adding e2e finalizer so DynamicHost is not deleted before we assert Draining/Drained")
			addDynamicHostE2EFinalizer(namespace, dynamicHostName)

			By("verifying no Runner is created (controller waits for driver to set state)")
			Consistently(func(g Gomega) {
				_, err := getRunnerOrErr(g, namespace, dynamicHostName)
				g.Expect(err).To(BeKubectlNotFound(), "expected no Runner when status.state is unset")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("does not create Runner when DynamicHost status.state is Pending", func() {
			applySpecification(dynamicHostYAML(dynamicHostName, namespace, dynamichostFlavor, dynamichostFlavor, dynamichostRootKeyName, "Pending"))

			By("verifying no Runner is created (controller waits for driver to set Ready)")
			Consistently(func(g Gomega) {
				_, err := getRunnerOrErr(g, namespace, dynamicHostName)
				g.Expect(err).To(BeKubectlNotFound(), "expected no Runner when status.state is Pending")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		When("DynamicHost state is Ready", func() {
			BeforeAll(func() {
				By("DynamicHost state is set to Ready")
				applySpecificationWithStatus(dynamicHostYAML(dynamicHostName, namespace, dynamichostFlavor, dynamichostFlavor, dynamichostRootKeyName, "Ready"))
			})

			It("sets finalizer", func() {
				By("verifying DynamicHost has the host-controller finalizer")
				Eventually(func(g Gomega) {
					h := getDynamicHost(g, namespace, dynamicHostName)
					g.Expect(h.Finalizers).To(ContainElement(constants.HostControllerFinalizer), "DynamicHost should have host-controller finalizer")
				}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			})

			It("creates the Runner", func() {
				By("waiting for Runner to be created")
				Eventually(func(g Gomega) {
					r := getRunner(g, namespace, dynamicHostName)
					g.Expect(r.Labels).To(HaveKeyWithValue(constants.HostLabel, dynamicHostName))
					g.Expect(r.Labels).To(HaveKeyWithValue(constants.RunnerTypeLabel, constants.RunnerTypeLabelDynamic))
					g.Expect(r.Spec.Flavor).To(Equal(dynamichostFlavor))
					g.Expect(r.Spec.Resources).To(HaveKey(corev1.ResourceName(dynamichostFlavor)))
					ownerRef := metav1.GetControllerOf(r)
					g.Expect(ownerRef).NotTo(BeNil(), "Runner should have an ownerReference to the DynamicHost")
					g.Expect(ownerRef.Kind).To(Equal("DynamicHost"))
					g.Expect(ownerRef.Name).To(Equal(dynamicHostName))
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			})

			It("reflects Runner ready and stopped counts in DynamicHost status", func() {
				By("waiting for Runner to become Ready (controller sets status)")
				Eventually(func(g Gomega) {
					r := getRunner(g, namespace, dynamicHostName)
					g.Expect(runner.IsReady(*r)).To(BeTrue(), "Runner should become Ready")
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

				By("waiting for DynamicHost status to reflect ready and stopped counts")
				Eventually(func(g Gomega) {
					h := getDynamicHost(g, namespace, dynamicHostName)
					g.Expect(h.Status.Runners.Ready).To(Equal(1), "status.runners.ready should be 1")
					g.Expect(h.Status.Runners.Stopped).To(Equal(0), "status.runners.stopped should be 0")
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			})

			It("reflects pipeline name in DynamicHost status when Runner has tekton.dev/pipeline label", func() {
				podName := "dynamichost-pipeline-pod"
				By("creating Pod with flavor annotation and tekton.dev/pipeline label (MAY propagates to Claim then Runner)")
				createPodWithFlavorAndLabels(podName, dynamichostTestNamespace, dynamichostFlavor, map[string]string{"tekton.dev/pipeline": dynamichostPipeline})
				DeferCleanup(func() { deletePod(dynamichostTestNamespace, podName) })

				By("waiting for Claim to be created and scheduled so Runner gets pipeline label from MAY")
				Eventually(func(g Gomega) {
					c := getClaim(g, dynamichostTestNamespace, podName)
					g.Expect(claim.IsClaimed(*c)).To(BeTrue(), "Claim should be Claimed")
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

				By("waiting for DynamicHost status.pipeline to reflect Runner pipeline label")
				Eventually(func(g Gomega) {
					h := getDynamicHost(g, namespace, dynamicHostName)
					g.Expect(h.Status.Pipeline).To(Equal(dynamichostPipeline), "status.pipeline should be set from Runner")
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			})

			When("DynamicHost's Runner is deleted", func() {

				It("does not recreate Runner", func() {
					By("deleting the DynamicHost's Runner")
					cmd := exec.Command("kubectl", "delete", "runner", dynamicHostName, "-n", namespace)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())

					By("waiting for Runner to be gone")
					Eventually(func(g Gomega) {
						_, err := getRunnerOrErr(g, namespace, dynamicHostName)
						g.Expect(err).To(BeKubectlNotFound(), "Runner should be deleted")
					}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

					By("verifying Runner is not recreated")
					Consistently(func(g Gomega) {
						_, err := getRunnerOrErr(g, namespace, dynamicHostName)
						g.Expect(err).To(BeKubectlNotFound(), "Runner should not be recreated")
					}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
				})

				It("marks DynamicHost as Draining", func() {
					By("verifying DynamicHost status.state is Draining or Drained (Runner was deleted)")
					Eventually(func(g Gomega) {
						h := getDynamicHost(g, namespace, dynamicHostName)
						g.Expect(h.Status.State).NotTo(BeNil(), "status.state should be set")
						g.Expect(*h.Status.State).To(Or(Equal(mayv1alpha1.HostActualStateDraining), Equal(mayv1alpha1.HostActualStateDrained)),
							"DynamicHost should be marked Draining or Drained when Runner is deleted")
					}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
				})
			})

			It("removes finalizer when DynamicHost is drained", func() {
				By("verifying DynamicHost status.state is Drained (Runner was deleted)")
				Eventually(func(g Gomega) {
					h := getDynamicHost(g, namespace, dynamicHostName)
					g.Expect(h.Status.State).NotTo(BeNil(), "status.state should be set")
					g.Expect(*h.Status.State).To(Equal(mayv1alpha1.HostActualStateDrained),
						"DynamicHost should be marked Drained when Runner is deleted")
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
				By("removing e2e finalizer so controller can remove its finalizer and delete the DynamicHost")
				removeDynamicHostE2EFinalizer(Default, namespace, dynamicHostName)
				By("waiting for DynamicHost to be deleted (finalizer removed)")
				Eventually(func(g Gomega) {
					_, err := getDynamicHostOrNotFound(g, namespace, dynamicHostName)
					g.Expect(err).To(HaveOccurred(), "DynamicHost should be deleted after finalizer removed")
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			})
		})
	})

	Context("When operator manually deletes DynamicHost", func() {
		It("deletes the Runner and removes DynamicHost", func() {
			hostName := "dynamichost-operator-delete"
			DeferCleanup(func() { deleteDynamicHost(namespace, hostName) })

			By("creating Ready DynamicHost with one Runner")
			applySpecificationWithStatus(dynamicHostYAML(hostName, namespace, dynamichostFlavor, dynamichostFlavor, dynamichostRootKeyName, "Ready"))

			By("waiting for Runner to be created")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, hostName)
				g.Expect(r.Name).To(Equal(hostName))
				g.Expect(r.Spec.Flavor).To(Equal(dynamichostFlavor))
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting DynamicHost to trigger finalizer")
			cmd := exec.Command("kubectl", "delete", "dynamichost", hostName, "-n", namespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Runner to be deleted by finalizer")
			Eventually(func(g Gomega) {
				_, err := getRunnerOrNotFound(g, namespace, hostName)
				g.Expect(err).To(HaveOccurred(), "Runner should be deleted")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for DynamicHost to be deleted (finalizer removed)")
			Eventually(func(g Gomega) {
				_, err := getDynamicHostOrNotFound(g, namespace, hostName)
				g.Expect(err).To(HaveOccurred(), "DynamicHost should be deleted after finalizer removed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
}
