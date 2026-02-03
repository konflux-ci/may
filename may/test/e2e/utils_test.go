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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/pod"
	"github.com/konflux-ci/may/test/utils"
)

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// getPod retrieves a Pod by name and namespace and decodes it. It fails the test via g if the get or decode fails.
func getPod(g Gomega, ns, name string) *corev1.Pod {
	cmd := exec.Command("kubectl", "get", "pod", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get pod %s/%s", ns, name)
	var p corev1.Pod
	g.Expect(json.Unmarshal([]byte(output), &p)).To(Succeed())
	return &p
}

// getClaim retrieves a Claim by name and namespace and decodes it. It fails the test via g if the get or decode fails.
func getClaim(g Gomega, ns, name string) *v1alpha1.Claim {
	cmd := exec.Command("kubectl", "get", "claim", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get claim %s/%s", ns, name)
	var c v1alpha1.Claim
	g.Expect(json.Unmarshal([]byte(output), &c)).To(Succeed())
	return &c
}

// getClaimOrNotFound returns the Claim if it exists, or an error if the resource is not found.
func getClaimOrNotFound(g Gomega, ns, name string) (*v1alpha1.Claim, error) {
	cmd := exec.Command("kubectl", "get", "claim", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	var c v1alpha1.Claim
	g.Expect(json.Unmarshal([]byte(output), &c)).To(Succeed())
	return &c, nil
}

// applySpecification applies the given manifest via kubectl apply -f - (stdin).
func applySpecification(specification string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(specification)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "error applying specification: %s", specification)
}

// applySpecificationStatus applies the status subresource from the given Specification via
// kubectl apply -f - --subresource=status. The Specification must include the resource
// identity (apiVersion, kind, metadata) and status.
func applySpecificationStatus(yaml string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-", "--subresource=status", "--server-side")
	cmd.Stdin = strings.NewReader(yaml)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed to apply status subresource")
}

// applySpecificationWithStatus applies the given manifest (spec), then applies the
// status subresource from the same YAML. The Specification must include spec and status
func applySpecificationWithStatus(specification string) {
	applySpecification(specification)
	applySpecificationStatus(specification)
}

// runnerYAML returns the YAML for a Runner with the given name, namespace, and flavor
// (no labels, so may's runner controller does not reconcile it unless the test adds them).
func runnerYAML(name, namespace, flavor string) string {
	return fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
spec:
  flavor: %s
  resources:
    %s: "1"
`, name, namespace, flavor, flavor)
}

// runnerYAMLWithReadyStatus returns the YAML for a Runner with the given name, namespace,
// and flavor, including status.conditions Ready=True so the scheduler can assign Claims.
func runnerYAMLWithReadyStatus(name, namespace, flavor string) string {
	return fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
spec:
  flavor: %s
  resources:
    %s: "1"
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: Ready
    lastTransitionTime: "2024-01-01T00:00:00Z"
`, name, namespace, flavor, flavor)
}

// runnerYAMLWithReadyStatusForBinder returns the YAML for a Runner with Ready status and
// the may.konflux-ci.dev/runner-user label required by RunnerBinder for user-dir in the
// Pod credentials Secret.
func runnerYAMLWithReadyStatusForBinder(name, namespace, flavor, runnerUser string) string {
	return fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
  labels:
    may.konflux-ci.dev/runner-user: %s
spec:
  flavor: %s
  resources:
    %s: "1"
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: Ready
    lastTransitionTime: "2024-01-01T00:00:00Z"
`, name, namespace, runnerUser, flavor, flavor)
}

// runnerSecretYAML returns the YAML for a Secret in the same namespace as the Runner,
// with the same name as the Runner, containing id_rsa (required by RunnerBinder to
// register the key with the OTP server). idRsaBase64 is the base64-encoded key content.
func runnerSecretYAML(runnerName, ns, idRsaBase64 string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
data:
  id_rsa: %s
`, runnerName, ns, idRsaBase64)
}

// getSecret retrieves a Secret by name and namespace via kubectl and decodes it.
func getSecret(g Gomega, ns, name string) *corev1.Secret {
	cmd := exec.Command("kubectl", "get", "secret", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get secret %s/%s", ns, name)
	var s corev1.Secret
	g.Expect(json.Unmarshal([]byte(output), &s)).To(Succeed())
	return &s
}

// deleteSecret deletes the Secret with the given name in the given namespace.
// It ignores "not found" and does not wait; used for test cleanup.
func deleteSecret(ns, name string) {
	cmd := exec.Command("kubectl", "delete", "secret", name, "-n", ns, "--ignore-not-found", "--wait=false")
	_, _ = utils.Run(cmd)
}

// otpRetrieverPodYAML returns the YAML for a Pod that mounts the credentials Secret
// (from RunnerBinder), POSTs the OTP to the OTP server, and verifies the response body
// equals expectedKey. The Pod succeeds (exit 0) only if the retrieved payload matches.
// If flavor is non-empty, the Pod gets the flavor annotation so the Claimer creates a
// Claim and the Pod goes through the full flow (Claim → Claimed → gate removed → scheduled).
func otpRetrieverPodYAML(podName, secretName, ns, expectedKey, flavor string) string {
	escaped := strings.ReplaceAll(expectedKey, "'", "'\"'\"'")
	var annotationsBlock string
	if flavor != "" {
		annotationsBlock = fmt.Sprintf("\n  annotations:\n    %s: \"1\"", pod.KueueFlavorLabelPrefix+flavor)
	}
	return fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s%s
spec:
  restartPolicy: Never
  containers:
  - name: retriever
    image: alpine:3.19
    command:
    - /bin/sh
    - -c
    - |
      set -e
      apk --no-interactive add curl
      curl -sS --fail-with-body --cacert /etc/secrets/otp/otp-ca -X POST --data-binary @/etc/secrets/otp/otp "$(cat /etc/secrets/otp/otp-server)" -o /tmp/retrieved-key
      echo -n '%s' | diff - /tmp/retrieved-key
      echo "Retrieved payload matches expected key"
    volumeMounts:
    - name: otp
      mountPath: /etc/secrets/otp
      readOnly: true
  volumes:
  - name: otp
    secret:
      secretName: %s
`, podName, ns, annotationsBlock, escaped, secretName)
}

// waitForPodSucceeded waits until the Pod has phase Succeeded.
func waitForPodSucceeded(ns, podName string) {
	cmd := exec.Command("kubectl", "wait", "pod/"+podName, "-n", ns,
		"--for", "jsonpath={.status.phase}=Succeeded",
		"--timeout", "2m")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "pod %s/%s should succeed", ns, podName)
}

// deleteRunner deletes the Runner with the given name from the manager namespace.
// It ignores "not found" and does not wait for completion; used for test cleanup.
func deleteRunner(name string) {
	cmd := exec.Command("kubectl", "delete", "runner", name, "-n", namespace, "--ignore-not-found", "--wait=false")
	_, _ = utils.Run(cmd)
}

// deletePod deletes the Pod with the given name in the given namespace.
// It ignores "not found" and does not wait for completion; used for test cleanup.
// Deleting the Pod causes the Claim (ownerReference) to be cascade-deleted.
func deletePod(ns, name string) {
	cmd := exec.Command("kubectl", "delete", "pod", name, "-n", ns, "--ignore-not-found", "--wait=false")
	_, _ = utils.Run(cmd)
}

// waitForClaimDeleted waits until the Claim with the given name no longer exists in the
// given namespace. Claims use the same name as their owning Pod. Fails the test if the
// Claim still exists after the timeout (30s, polling every 2s).
func waitForClaimDeleted(ns, claimName string) {
	Eventually(func(g Gomega) {
		_, err := getClaimOrNotFound(g, ns, claimName)
		g.Expect(err).To(HaveOccurred(), "expected Claim %s/%s to be deleted", ns, claimName)
	}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
}

// createPodWithFlavor creates a Pod in the given namespace with the kueue flavor annotation
// (kueue.konflux-ci.dev/requests-<flavor>). The Claimer will create a Claim for this Pod
// in tenant namespaces; the Claim has the same name as the Pod.
func createPodWithFlavor(podName, ns, flavor string) {
	createPodWithFlavorAndLabels(podName, ns, flavor, nil)
}

// createPodWithFlavorAndLabels is like createPodWithFlavor but also sets metadata.labels
// from the given map (e.g. tekton.dev/pipeline for MAY to propagate to Claim and Runner).
func createPodWithFlavorAndLabels(podName, ns, flavor string, labels map[string]string) {
	flavorAnnotation := pod.KueueFlavorLabelPrefix + flavor
	overrides := fmt.Sprintf(`{"metadata":{"annotations":{"%s":"1"}}}`, flavorAnnotation)
	if len(labels) > 0 {
		labelsJSON, err := json.Marshal(labels)
		Expect(err).NotTo(HaveOccurred())
		overrides = fmt.Sprintf(`{"metadata":{"annotations":{"%s":"1"},"labels":%s}}`, flavorAnnotation, string(labelsJSON))
	}
	cmd := exec.Command("kubectl", "run", podName,
		"--image=registry.k8s.io/pause:3.9",
		"--restart=Never",
		"-n", ns,
		"--overrides", overrides,
	)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

// getRunner retrieves a Runner by name and namespace via kubectl and decodes it.
// It fails the test via g if the get or JSON decode fails (e.g. Runner not found).
func getRunner(g Gomega, ns, name string) *v1alpha1.Runner {
	cmd := exec.Command("kubectl", "get", "runner", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get runner %s/%s", ns, name)
	var r v1alpha1.Runner
	g.Expect(json.Unmarshal([]byte(output), &r)).To(Succeed())
	return &r
}

// getRunnerOrNotFound returns the Runner if it exists; returns an error if the resource
// is not found (e.g. after finalizer deleted it). Used to assert Runner deletion.
func getRunnerOrNotFound(g Gomega, ns, name string) (*v1alpha1.Runner, error) {
	cmd := exec.Command("kubectl", "get", "runner", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	var r v1alpha1.Runner
	g.Expect(json.Unmarshal([]byte(output), &r)).To(Succeed())
	return &r, nil
}

// staticHostYAML returns the YAML for a StaticHost with the given spec. statusState controls
// status.state: use "" for no status (controller waits for driver), "Pending", or "Ready".
// rootKeyName is required by the API; use a placeholder if the test does not need it.
func staticHostYAML(name, ns, flavor string, instances int, resourceKey, rootKeyName, statusState string) string {
	specAndMeta := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: StaticHost
metadata:
  name: %s
  namespace: %s
spec:
  flavor: %s
  rootKey:
    name: %s
  runners:
    instances: %d
    resources:
      %s: "1"
`, name, ns, flavor, rootKeyName, instances, resourceKey)
	if statusState == "" {
		return specAndMeta
	}
	return specAndMeta + fmt.Sprintf("status:\n  state: %s\n", statusState)
}

// runnerStatusReadyYAML returns a minimal Runner YAML (metadata + status) for patching
// the status subresource so the Runner has Ready=True. Used to drive StaticHost
// status.runners.ready / status.runners.stopped updates.
func runnerStatusReadyYAML(name, ns string) string {
	return fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: Runner
metadata:
  name: %s
  namespace: %s
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: Ready
    lastTransitionTime: "2024-01-01T00:00:00Z"
`, name, ns)
}

// getStaticHost retrieves a StaticHost by name and namespace via kubectl and decodes it.
func getStaticHost(g Gomega, ns, name string) *v1alpha1.StaticHost {
	cmd := exec.Command("kubectl", "get", "statichost", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get statichost %s/%s", ns, name)
	var h v1alpha1.StaticHost
	g.Expect(json.Unmarshal([]byte(output), &h)).To(Succeed())
	return &h
}

// getStaticHostOrNotFound returns the StaticHost if it exists; returns an error if the resource
// is not found (e.g. after finalizer removed and resource deleted).
func getStaticHostOrNotFound(g Gomega, ns, name string) (*v1alpha1.StaticHost, error) {
	cmd := exec.Command("kubectl", "get", "statichost", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	var h v1alpha1.StaticHost
	g.Expect(json.Unmarshal([]byte(output), &h)).To(Succeed())
	return &h, nil
}

// deleteStaticHost deletes the StaticHost; used for test cleanup. Runners are cascade-deleted
// or removed by the controller finalizer.
func deleteStaticHost(ns, name string) {
	cmd := exec.Command("kubectl", "delete", "statichost", name, "-n", ns, "--ignore-not-found")
	_, _ = utils.Run(cmd)
}

// dynamicHostYAML returns the YAML for a DynamicHost with the given spec. statusState controls
// status.state: use "" for no status (controller waits for driver), "Pending", or "Ready".
// DynamicHost has a single Runner (same name as the host); spec.runner.resources is used.
func dynamicHostYAML(name, ns, flavor, resourceKey, rootKeyName, statusState string) string {
	specAndMeta := fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: DynamicHost
metadata:
  name: %s
  namespace: %s
spec:
  flavor: %s
  rootKey:
    name: %s
  runner:
    resources:
      %s: "1"
`, name, ns, flavor, rootKeyName, resourceKey)
	if statusState == "" {
		return specAndMeta
	}
	return specAndMeta + fmt.Sprintf("status:\n  state: %s\n", statusState)
}

// dynamicHostAutoscalerYAML returns the YAML for a DynamicHostAutoscaler with the given name,
// namespace, flavor, and template (minimal DynamicHost spec: flavor, rootKey, status Ready, runner resources).
// The provisioner creates DynamicHosts named like the Claim; it overwrites spec.rootKey.name with the host name.
func dynamicHostAutoscalerYAML(name, ns, flavor, resourceKey, rootKeyName string) string {
	return fmt.Sprintf(`apiVersion: may.konflux-ci.dev/v1alpha1
kind: DynamicHostAutoscaler
metadata:
  name: %s
  namespace: %s
spec:
  flavor: %s
  template:
    spec:
      flavor: %s
      rootKey:
        name: %s
      status: Ready
      runner:
        resources:
          %s: "1"
`, name, ns, flavor, flavor, rootKeyName, resourceKey)
}

// getDynamicHostAutoscaler retrieves a DynamicHostAutoscaler by name and namespace via kubectl and decodes it.
func getDynamicHostAutoscaler(g Gomega, ns, name string) *v1alpha1.DynamicHostAutoscaler {
	cmd := exec.Command("kubectl", "get", "dynamichostautoscaler", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get dynamichostautoscaler %s/%s", ns, name)
	var a v1alpha1.DynamicHostAutoscaler
	g.Expect(json.Unmarshal([]byte(output), &a)).To(Succeed())
	return &a
}

// deleteDynamicHostAutoscaler deletes the DynamicHostAutoscaler; used for test cleanup.
func deleteDynamicHostAutoscaler(ns, name string) {
	cmd := exec.Command("kubectl", "delete", "dynamichostautoscaler", name, "-n", ns, "--ignore-not-found")
	_, _ = utils.Run(cmd)
}

// getDynamicHost retrieves a DynamicHost by name and namespace via kubectl and decodes it.
func getDynamicHost(g Gomega, ns, name string) *v1alpha1.DynamicHost {
	cmd := exec.Command("kubectl", "get", "dynamichost", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get dynamichost %s/%s", ns, name)
	var h v1alpha1.DynamicHost
	g.Expect(json.Unmarshal([]byte(output), &h)).To(Succeed())
	return &h
}

// getDynamicHostOrNotFound returns the DynamicHost if it exists; returns an error if the resource
// is not found (e.g. after finalizer removed and resource deleted).
func getDynamicHostOrNotFound(g Gomega, ns, name string) (*v1alpha1.DynamicHost, error) {
	cmd := exec.Command("kubectl", "get", "dynamichost", name, "-n", ns, "-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	var h v1alpha1.DynamicHost
	g.Expect(json.Unmarshal([]byte(output), &h)).To(Succeed())
	return &h, nil
}

// deleteDynamicHost deletes the DynamicHost; used for test cleanup. The Runner is cascade-deleted
// or removed by the controller finalizer.
func deleteDynamicHost(ns, name string) {
	cmd := exec.Command("kubectl", "delete", "dynamichost", name, "-n", ns, "--ignore-not-found")
	_, _ = utils.Run(cmd)
}

// addDynamicHostE2EFinalizer adds the e2e-test finalizer to the DynamicHost so it is not
// fully deleted until we remove it (allows asserting Draining/Drained before controller cleanup).
func addDynamicHostE2EFinalizer(ns, name string) {
	cmd := exec.Command("kubectl", "patch", "dynamichost", name, "-n", ns, "--type=json",
		"-p", fmt.Sprintf(`[{"op": "add", "path": "/metadata/finalizers/-", "value": %q}]`, e2eTestFinalizer))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

// removeDynamicHostE2EFinalizer removes the e2e-test finalizer from the DynamicHost so the
// controller can complete deletion. No-op if the host or finalizer is already gone.
func removeDynamicHostE2EFinalizer(g Gomega, ns, name string) {
	h, err := getDynamicHostOrNotFound(g, ns, name)
	if err != nil {
		return // already deleted
	}
	var newFinalizers []string
	for _, f := range h.Finalizers {
		if f != e2eTestFinalizer {
			newFinalizers = append(newFinalizers, f)
		}
	}
	value, err := json.Marshal(newFinalizers)
	g.Expect(err).NotTo(HaveOccurred())
	cmd := exec.Command("kubectl", "patch", "dynamichost", name, "-n", ns, "--type=json",
		"-p", fmt.Sprintf(`[{"op": "replace", "path": "/metadata/finalizers", "value": %s}]`, string(value)))
	_, err = utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred())
}
