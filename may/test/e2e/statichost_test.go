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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/test/utils"
)

// StaticHostContexts registers the StaticHost (in may) test contexts.
// Ordered tests share one StaticHost: the first creates it and Runners, the second
// verifies status reflects Runner counts.
func StaticHostContexts() {
	Context("StaticHost (in may)", Ordered, func() {
		var staticHostName string = "statichost-full-workflow"
		var instances int = 2

		BeforeAll(func() {
			By("creating namespace for StaticHost tests")
			cmd := exec.Command("kubectl", "create", "namespace", statichostTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			By("labeling the namespace as tenant (for Claimer to create Claims from Pods)")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", statichostTestNamespace,
				constants.TenantNamespaceLabelKey+"="+constants.TenantNamespaceLabelValue)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting StaticHost and its Runners")
			deleteStaticHost(namespace, staticHostName)

			By("deleting StaticHost test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", statichostTestNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("does not create Runners when StaticHost status.state is unset", func() {
			applySpecification(staticHostYAML(staticHostName, namespace, statichostFlavor, instances, statichostFlavor, statichostRootKeyName, ""))

			By("verifying no Runners are created (controller waits for driver to set state)")
			runner0Name := fmt.Sprintf("%s-0", staticHostName)
			Consistently(func(g Gomega) {
				_, err := getRunnerOrNotFound(g, namespace, runner0Name)
				g.Expect(err).To(HaveOccurred(), "expected no Runner when status.state is unset")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("does not create Runners when StaticHost status.state is Pending", func() {
			applySpecification(staticHostYAML(staticHostName, namespace, statichostFlavor, instances, statichostFlavor, statichostRootKeyName, "Pending"))

			By("verifying no Runners are created (controller waits for driver to set Ready)")
			runner0Name := fmt.Sprintf("%s-0", staticHostName)
			Consistently(func(g Gomega) {
				_, err := getRunnerOrNotFound(g, namespace, runner0Name)
				g.Expect(err).To(HaveOccurred(), "expected no Runner when status.state is Pending")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("creates Runners when StaticHost state is Ready", func() {
			applySpecificationWithStatus(staticHostYAML(staticHostName, namespace, statichostFlavor, instances, statichostFlavor, statichostRootKeyName, "Ready"))

			By("waiting for Runners to be created (one per instance index)")
			for i := range instances {
				runnerName := fmt.Sprintf("%s-%d", staticHostName, i)
				runnerID := fmt.Sprintf("%d", i)
				Eventually(func(g Gomega) {
					r := getRunner(g, namespace, runnerName)
					g.Expect(r.Labels).To(HaveKeyWithValue(constants.HostLabel, staticHostName))
					g.Expect(r.Labels).To(HaveKeyWithValue(constants.RunnerIdLabel, runnerID))
					g.Expect(r.Labels).To(HaveKeyWithValue(constants.RunnerTypeLabel, constants.RunnerTypeLabelStatic))
					g.Expect(r.Spec.Flavor).To(Equal(statichostFlavor))
					g.Expect(r.Spec.Resources).To(HaveKey(corev1.ResourceName(statichostFlavor)))
					ownerRef := metav1.GetControllerOf(r)
					g.Expect(ownerRef).NotTo(BeNil(), "Runner should have an ownerReference to the StaticHost")
					g.Expect(ownerRef.Kind).To(Equal("StaticHost"))
					g.Expect(ownerRef.Name).To(Equal(staticHostName))
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			}
		})

		It("sets finalizer on StaticHost when reconciled", func() {
			By("verifying StaticHost has the host-controller finalizer")
			Eventually(func(g Gomega) {
				h := getStaticHost(g, namespace, staticHostName)
				g.Expect(h.Finalizers).To(ContainElement(constants.HostControllerFinalizer), "StaticHost should have host-controller finalizer")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("reflects Runner ready and stopped counts in StaticHost status when Runners are Ready", func() {
			By("waiting for Runners to become Ready (controller sets status)")
			for i := range instances {
				runnerName := fmt.Sprintf("%s-%d", staticHostName, i)
				Eventually(func(g Gomega) {
					r := getRunner(g, namespace, runnerName)
					g.Expect(runner.IsReady(*r)).To(BeTrue(), "Runner %s should become Ready", runnerName)
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			}

			By("waiting for StaticHost status to reflect ready and stopped counts")
			Eventually(func(g Gomega) {
				h := getStaticHost(g, namespace, staticHostName)
				g.Expect(h.Status.Runners.Ready).To(Equal(2), "status.runners.ready should be 2")
				g.Expect(h.Status.Runners.Stopped).To(Equal(0), "status.runners.stopped should be 0")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("reflects pipeline names in StaticHost status when Runners have tekton.dev/pipeline labels", func() {
			podA := "statichost-pipeline-pod-a"
			podB := "statichost-pipeline-pod-b"
			By("creating Pods with flavor annotation and tekton.dev/pipeline label (MAY propagates to Claim then Runner)")
			createPodWithFlavorAndLabels(podA, statichostTestNamespace, statichostFlavor, map[string]string{"tekton.dev/pipeline": statichostPipelineA})
			createPodWithFlavorAndLabels(podB, statichostTestNamespace, statichostFlavor, map[string]string{"tekton.dev/pipeline": statichostPipelineB})

			By("waiting for Claims to be created and scheduled so Runners get pipeline label from MAY")
			Eventually(func(g Gomega) {
				cA := getClaim(g, statichostTestNamespace, podA)
				g.Expect(claim.IsClaimed(*cA)).To(BeTrue(), "Claim for pod-a should be Claimed")
				cB := getClaim(g, statichostTestNamespace, podB)
				g.Expect(claim.IsClaimed(*cB)).To(BeTrue(), "Claim for pod-b should be Claimed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for StaticHost status.pipelines to reflect Runner pipeline labels (sorted)")
			Eventually(func(g Gomega) {
				h := getStaticHost(g, namespace, staticHostName)
				g.Expect(h.Status.Pipelines).To(Equal([]string{statichostPipelineA, statichostPipelineB}), "status.pipelines should be sorted pipeline names from Runners")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("deleting pods to simulate their completion")
			deletePod(statichostTestNamespace, podA)
			deletePod(statichostTestNamespace, podB)
		})

		It("creates Runners when StaticHost runners are increased", func() {
			By(fmt.Sprintf("scaling StaticHost to %d instances", instances+1))
			applySpecificationWithStatus(staticHostYAML(staticHostName, namespace, statichostFlavor, instances+1, statichostFlavor, statichostRootKeyName, "Ready"))

			By("waiting for Runner 2 to be created")
			runner2Name := fmt.Sprintf("%s-2", staticHostName)
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runner2Name)
				g.Expect(r.Labels).To(HaveKeyWithValue(constants.RunnerIdLabel, "2"))
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("deletes excess Runners when StaticHost runners are reduced", func() {
			By(fmt.Sprintf("scaling StaticHost back to %d instances", instances))
			applySpecificationWithStatus(staticHostYAML(staticHostName, namespace, statichostFlavor, instances, statichostFlavor, statichostRootKeyName, "Ready"))

			By("waiting for Runner 2 to be deleted (runner-id outside [0..instances-1])")
			runner2Name := fmt.Sprintf("%s-2", staticHostName)
			Eventually(func(g Gomega) {
				_, err := getRunnerOrNotFound(g, namespace, runner2Name)
				g.Expect(err).To(HaveOccurred(), "Runner %s should be deleted", runner2Name)
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying Runners 0 and 1 still exist")
			Eventually(func(g Gomega) {
				getRunner(g, namespace, fmt.Sprintf("%s-0", staticHostName))
				getRunner(g, namespace, fmt.Sprintf("%s-1", staticHostName))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("deletes all Runners and removes finalizer when StaticHost is deleted", func() {
			By("deleting StaticHost to trigger finalizer")
			cmd := exec.Command("kubectl", "delete", "statichost", staticHostName, "-n", namespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Runners to be deleted by finalizer")
			for i := range instances {
				runnerName := fmt.Sprintf("%s-%d", staticHostName, i)
				Eventually(func(g Gomega) {
					_, err := getRunnerOrNotFound(g, namespace, runnerName)
					g.Expect(err).To(HaveOccurred(), "Runner %s should be deleted", runnerName)
				}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
			}

			By("waiting for StaticHost to be deleted (finalizer removed)")
			Eventually(func(g Gomega) {
				_, err := getStaticHostOrNotFound(g, namespace, staticHostName)
				g.Expect(err).To(HaveOccurred(), "StaticHost should be deleted after finalizer removed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
}
