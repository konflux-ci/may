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
	v1 "k8s.io/api/core/v1"

	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
	"github.com/konflux-ci/may/test/utils"
)

// RunnerLifecycleContexts registers the Runner Lifecycle (Provisioner) test contexts.
// Tests do not involve StaticHost; Runners are created by the test (simulating an external driver).
func RunnerLifecycleContexts() {
	Context("Runner Lifecycle (Provisioner)", Ordered, func() {
		BeforeAll(func() {
			By("creating tenant namespace for Runner Lifecycle tests")
			cmd := exec.Command("kubectl", "create", "namespace", runnerLifecycleNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", runnerLifecycleNamespace, "konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting Runner Lifecycle test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", runnerLifecycleNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("leaves Runner with no condition untouched (may waits for driver to set Ready)", func() {
			runnerName := "runner-no-condition"
			defer deleteRunner(runnerName)
			applySpecification(runnerYAML(runnerName, namespace, runnerLifecycleFlavor))

			By("verifying Runner still has no Ready condition after a short period")
			Consistently(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsReadySet(*r)).To(BeFalse(), "may must not set Ready on a Runner without runner-type label; may waits for driver")
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("reflects reservation when Runner has spec.inUseBy set", func() {
			runnerName := "runner-reserved"
			podName := "pod-runner-reserved"
			applySpecificationWithStatus(runnerYAMLWithReadyStatus(runnerName, namespace, runnerLifecycleFlavor))
			defer deleteRunner(runnerName)
			defer deletePod(runnerLifecycleNamespace, podName)

			createPodWithFlavor(podName, runnerLifecycleNamespace, runnerLifecycleFlavor)

			By("waiting for Claim to be Claimed")
			Eventually(func(g Gomega) {
				c := getClaim(g, runnerLifecycleNamespace, podName)
				g.Expect(claim.IsClaimed(*c)).To(BeTrue())
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying Runner status reflects reservation (spec.inUseBy set)")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsReserved(*r)).To(BeTrue())
				g.Expect(r.Spec.InUseBy.Name).To(Equal(podName))
				g.Expect(r.Spec.InUseBy.Namespace).To(Equal(runnerLifecycleNamespace))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("runs provisioning hooks and updates status", func() {
			runnerName := "runner-provisioning-hooks"
			defer deleteRunner(runnerName)

			applyRunnerWithProvisioningHook(runnerName, runnerLifecycleFlavor)

			By("waiting for Runner to get Initializing and hook pod to be created")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsInitializing(*r)).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			hookPodPrefix := "p-" + runnerName + "-"
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-o", "name")
				out, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring(hookPodPrefix), "provisioning hook pod should be created")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for provisioning hook pod to succeed and Runner to become Ready with hooksStatus updated")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsReady(*r)).To(BeTrue())
				g.Expect(r.Status.HooksStatus.Provisioning).NotTo(BeEmpty())
				g.Expect(r.Status.HooksStatus.Provisioning[0].Phase).To(Equal(v1.PodPhase("Succeeded")))
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("initializes Runner without hooks directly to Ready", func() {
			runnerName := "runner-no-hooks"
			defer deleteRunner(runnerName)

			applyRunnerWithoutHooks(runnerName, runnerLifecycleFlavor)

			By("waiting for Runner to become Ready without any hook pods")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsReady(*r)).To(BeTrue())
				g.Expect(r.Status.HooksStatus.Provisioning).To(BeEmpty(), "no provisioning hook pods should be observed")
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("runs multiple provisioning hooks sequentially", func() {
			runnerName := "runner-multi-hooks"
			defer deleteRunner(runnerName)

			applyRunnerWithMultipleProvisioningHooks(runnerName, runnerLifecycleFlavor)

			By("waiting for first hook pod to be created")
			firstHookPod := "p-" + runnerName + "-setup-first"
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", firstHookPod, "-n", namespace, "-o", "name")
				out, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring(firstHookPod))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying second hook pod is not created while first is running")
			secondHookPod := "p-" + runnerName + "-setup-second"
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", secondHookPod, "-n", namespace, "-o", "name")
				_, err := utils.Run(cmd)
				g.Expect(err).To(BeKubectlNotFound(), "second hook pod should not exist while first is running")
			}).WithTimeout(3 * time.Second).WithPolling(1 * time.Second).Should(Succeed())

			By("waiting for all hooks to complete and Runner to become Ready")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsReady(*r)).To(BeTrue())
				g.Expect(r.Status.HooksStatus.Provisioning).To(HaveLen(2))
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("marks Runner as InitializationFailed when provisioning hook fails", func() {
			runnerName := "runner-init-failed"
			defer deleteRunner(runnerName)

			applyRunnerWithFailingProvisioningHook(runnerName, runnerLifecycleFlavor)

			By("waiting for Runner to get Initializing and hook pod to be created")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsInitializing(*r)).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for provisioning hook to fail and Runner to get InitializationFailed")
			Eventually(func(g Gomega) {
				r := getRunner(g, namespace, runnerName)
				g.Expect(runner.IsInitializationFailed(*r)).To(BeTrue())
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("runs cleanup hooks and updates status before Runner is deleted", func() {
			runnerName := "runner-cleanup-hooks"
			applyRunnerWithCleanupHook(runnerName, runnerLifecycleFlavor)

			By("patching Runner to Ready so we can trigger deletion and cleanup")
			statusPatch := `{"status":{"conditions":[{"type":"Ready","status":"True","reason":"Ready","message":"Ready","lastTransitionTime":"2024-01-01T00:00:00Z"}]}}`
			cmd := exec.Command("kubectl", "patch", "runner", runnerName, "-n", namespace, "--type=merge", "-p", statusPatch, "--subresource=status")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("deleting Runner to trigger cleanup hooks")
			deleteRunner(runnerName)

			By("waiting for Runner to enter Cleaning and cleanup hook pod to be created")
			Eventually(func(g Gomega) {
				r, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(runner.IsCleaning(*r)).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			cleanupPodPrefix := "c-" + runnerName + "-"
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-o", "name")
				out, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring(cleanupPodPrefix))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for cleanup to complete and Runner to be deleted")
			Eventually(func(g Gomega) {
				_, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).To(BeKubectlNotFound(), "Runner should be deleted after cleanup hooks succeed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("exposes may_runner_initialized metric after initialization", func() {
			By("creating a curl pod to scrape fresh metrics from the controller")
			podName := createMetricsCurlPod()
			defer deletePod(namespace, podName)

			By("waiting for the curl pod to complete")
			waitForPodSucceeded(namespace, podName)

			By("verifying the metrics output contains runner initialization counters")
			cmd := exec.Command("kubectl", "logs", podName, "-n", namespace)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring(`may_runner_initialized{result="success",type="static"}`))
			Expect(output).To(ContainSubstring(`may_runner_initialized{result="failure",type="static"}`))
		})

		It("does not delete Runner when cleanup fails", func() {
			runnerName := "runner-cleanup-failed"
			applyRunnerWithFailingCleanupHook(runnerName, runnerLifecycleFlavor)
			defer func() {
				By("removing finalizers on Runner so it can be deleted during test cleanup")
				cmd := exec.Command("kubectl", "patch", "runner", runnerName, "-n", namespace, "--type=json", "-p", `[{"op": "replace", "path": "/metadata/finalizers", "value": []}]`)
				_, _ = utils.Run(cmd)
			}()

			By("patching Runner to Ready")
			statusPatch := `{"status":{"conditions":[{"type":"Ready","status":"True","reason":"Ready","message":"Ready","lastTransitionTime":"2024-01-01T00:00:00Z"}]}}`
			cmd := exec.Command("kubectl", "patch", "runner", runnerName, "-n", namespace, "--type=merge", "-p", statusPatch, "--subresource=status")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("deleting Runner to trigger cleanup")
			cmd = exec.Command("kubectl", "delete", "runner", runnerName, "-n", namespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)

			By("waiting for Runner to enter Cleaning and cleanup hook pod to be created")
			Eventually(func(g Gomega) {
				r, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(runner.IsCleaning(*r)).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("waiting for cleanup hook to fail and Runner to get CleaningFailed (Runner must not be deleted)")
			Eventually(func(g Gomega) {
				r, err := getRunnerOrErr(g, namespace, runnerName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(runner.IsNotReadyWithReason(*r, runner.ConditionReasonCleaningFailed)).To(BeTrue())
				g.Expect(r.DeletionTimestamp).NotTo(BeNil(), "Runner should still exist with deletion timestamp")
				g.Expect(r.Finalizers).To(ContainElement("may.konflux-ci.dev/runner-controller"), "finalizer should remain when cleanup failed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
}

// applyRunnerWithProvisioningHook creates a Runner with runner-type=static and one provisioning hook
// that runs "true" (exits 0). May will set Initializing and run the hook, then set Ready.
func applyRunnerWithProvisioningHook(name, flavor string) {
	yaml := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  flavor: %s
  resources:
    %s: "1"
  hooks:
    provisioning:
    - name: setup
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsNonRoot: true
            seccompProfile:
              type: RuntimeDefault
          containers:
          - name: main
            image: busybox:1.36
            command: ["true"]
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop: ["ALL"]
              runAsNonRoot: true
              runAsUser: 1000
              seccompProfile:
                type: RuntimeDefault
`, name, namespace, constants.RunnerTypeLabel, runnerTypeStatic, flavor, flavor)
	applySpecification(yaml)
}

// applyRunnerWithCleanupHook creates a Runner with runner-type=static and one cleanup hook
// that runs "true" (exits 0).
func applyRunnerWithCleanupHook(name, flavor string) {
	yaml := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  flavor: %s
  resources:
    %s: "1"
  hooks:
    cleanup:
    - name: teardown
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsNonRoot: true
            seccompProfile:
              type: RuntimeDefault
          containers:
          - name: main
            image: busybox:1.36
            command: ["true"]
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop: ["ALL"]
              runAsNonRoot: true
              runAsUser: 1000
              seccompProfile:
                type: RuntimeDefault
`, name, namespace, constants.RunnerTypeLabel, runnerTypeStatic, flavor, flavor)
	applySpecification(yaml)
}

// applyRunnerWithFailingProvisioningHook creates a Runner with runner-type=static and one provisioning hook
// that runs "exit 1" so initialization fails and the Runner is marked as InitializationFailed.
func applyRunnerWithFailingProvisioningHook(name, flavor string) {
	yaml := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  flavor: %s
  resources:
    %s: "1"
  hooks:
    provisioning:
    - name: setup
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsNonRoot: true
            seccompProfile:
              type: RuntimeDefault
          containers:
          - name: main
            image: busybox:1.36
            command: ["sh", "-c", "exit 1"]
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop: ["ALL"]
              runAsNonRoot: true
              runAsUser: 1000
              seccompProfile:
                type: RuntimeDefault
`, name, namespace, constants.RunnerTypeLabel, runnerTypeStatic, flavor, flavor)
	applySpecification(yaml)
}

// applyRunnerWithFailingCleanupHook creates a Runner with runner-type=static and one cleanup hook
// that runs "exit 1" so cleanup fails and the Runner is not deleted.
func applyRunnerWithFailingCleanupHook(name, flavor string) {
	yaml := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  flavor: %s
  resources:
    %s: "1"
  hooks:
    cleanup:
    - name: teardown
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsNonRoot: true
            seccompProfile:
              type: RuntimeDefault
          containers:
          - name: main
            image: busybox:1.36
            command: ["sh", "-c", "exit 1"]
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop: ["ALL"]
              runAsNonRoot: true
              runAsUser: 1000
              seccompProfile:
                type: RuntimeDefault
`, name, namespace, constants.RunnerTypeLabel, runnerTypeStatic, flavor, flavor)
	applySpecification(yaml)
}

// applyRunnerWithoutHooks creates a Runner with runner-type=static and no hooks.
// The provisioner initializes it directly to Ready without creating any hook pods.
func applyRunnerWithoutHooks(name, flavor string) {
	yaml := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  flavor: %s
  resources:
    %s: "1"
`, name, namespace, constants.RunnerTypeLabel, runnerTypeStatic, flavor, flavor)
	applySpecification(yaml)
}

// applyRunnerWithMultipleProvisioningHooks creates a Runner with runner-type=static and two
// provisioning hooks that run sequentially. The first hook sleeps briefly so the test can
// observe that the second hook is not created until the first completes.
func applyRunnerWithMultipleProvisioningHooks(name, flavor string) {
	yaml := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  flavor: %s
  resources:
    %s: "1"
  hooks:
    provisioning:
    - name: setup-first
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsNonRoot: true
            seccompProfile:
              type: RuntimeDefault
          containers:
          - name: main
            image: busybox:1.36
            command: ["sh", "-c", "sleep 5"]
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop: ["ALL"]
              runAsNonRoot: true
              runAsUser: 1000
              seccompProfile:
                type: RuntimeDefault
    - name: setup-second
      template:
        spec:
          restartPolicy: Never
          securityContext:
            runAsNonRoot: true
            seccompProfile:
              type: RuntimeDefault
          containers:
          - name: main
            image: busybox:1.36
            command: ["true"]
            securityContext:
              allowPrivilegeEscalation: false
              capabilities:
                drop: ["ALL"]
              runAsNonRoot: true
              runAsUser: 1000
              seccompProfile:
                type: RuntimeDefault
`, name, namespace, constants.RunnerTypeLabel, runnerTypeStatic, flavor, flavor)
	applySpecification(yaml)
}
