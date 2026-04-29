//go:build e2e
// +build e2e

/*
Copyright 2025.

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
	"context"
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
	"k8s.io/apimachinery/pkg/util/intstr"

	_ "embed"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/drivers/incluster/test/utils"
)

// namespace where the project is deployed in
const namespace = "may-system"

// serviceAccountName created for the project
const serviceAccountName = "incluster-incluster-driver"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "incluster-incluster-driver-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "incluster-metrics-binding"

//go:embed assets/statichost_aws_host_arm64.yaml
var staticHostAwsHostArm64 string

//go:embed assets/dynamichost_aws_host_arm64.yaml
var dynamicHostAwsHostArm64 string

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")
		cmd = exec.Command("make", "-C", "../../may", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")
		cmd = exec.Command("kubectl", "get", "crd", "statichosts.may.konflux-ci.dev")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to find the StaticHost CRD")

		By("deploying the incluster-driver")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the incluster-driver")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace. When ENABLE_COVERAGE is set, skip cleanup so that coverport can
	// collect coverage data from the still-running pods before the Kind cluster is deleted.
	AfterAll(func() {
		if os.Getenv("ENABLE_COVERAGE") == "true" {
			By("skipping cleanup: ENABLE_COVERAGE is set, pods kept running for coverage collection")
			return
		}

		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the incluster-driver")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
		cmd = exec.Command("make", "-C", "../../may", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the incluster-driver pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the incluster-driver pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=incluster-driver",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve incluster-driver pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("incluster-driver"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect incluster-driver pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--dry-run=client",
				"-o",
				"yaml",
				"--clusterrole=incluster-metrics-reader",
				"--dry-run=client",
				"--output=yaml",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			crb, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to generate ClusterRoleBinding")
			cmd = exec.Command("kubectl", "apply", "-f", "-", "--server-side")
			cmd.Stdin = strings.NewReader(crb)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput, err := getMetricsOutput()
		// Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
		var _ = Describe("StaticHost", Ordered, func() {

			var namespace = "e2e-static-host"

			When("a StaticHost is created", func() {
				BeforeAll(func() {
					cmd := exec.Command("kubectl", "create", "namespace", namespace)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("kubectl", "apply", "--namespace", namespace, "-f", "-")
					cmd.Stdin = strings.NewReader(staticHostAwsHostArm64)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())
				})

				It("sets the Pending state", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "statichosts.may.konflux-ci.dev", "aws-host-arm64",
							"-n", namespace,
							"-o", "go-template="+
								"{{ .status.state }}"+
								"{{ \"\\n\" }}",
						)
						sh, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						ll := utils.GetNonEmptyLines(sh)
						g.Expect(ll).To(HaveLen(1))
						g.Expect(ll[0]).To(Equal(maykonfluxcidevv1alpha1.HostActualStatePending))
					})
				})

				It("creates the secret", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "secrets", "ssh-aws-host-arm64-key",
							"-n", namespace,
							"-o", "json",
						)
						js, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						s := corev1.Secret{}
						Expect(json.Unmarshal([]byte(js), &s)).To(Succeed())
						Expect(s.Data).To(
							And(
								Not(BeEmpty()),
								HaveKey("id_rsa"),
								HaveKey("id_rsa.pub"),
							))
					})
				})

				It("creates the pod", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "pods", "aws-host-arm64",
							"-n", namespace,
							"-o", "json",
						)
						jp, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						p := corev1.Pod{}
						Expect(json.Unmarshal([]byte(jp), &p)).To(Succeed())
						Expect(p.Labels).To(And(
							Not(BeEmpty()),
							HaveKeyWithValue("may.konflux-ci.dev/host", "aws-host-arm64"),
						))
						Expect(p.Spec.Containers).To(HaveLen(1))
						Expect(p.Spec.Containers[0].Env).To(
							And(
								Satisfy(func(e corev1.EnvVar) bool {
									return e.Name == "PUBLIC_KEY" &&
										e.ValueFrom != nil &&
										e.ValueFrom.SecretKeyRef == nil &&
										e.ValueFrom.SecretKeyRef.Key == "id_rsa.pub" &&
										e.ValueFrom.SecretKeyRef.LocalObjectReference.Name == "ssh-aws-host-arm64-key"
								}),
								Satisfy(func(e corev1.EnvVar) bool {
									return e.Name == "USER_NAME" && e.Value == "admin"
								}),
							))
						Expect(p.Spec.Containers[0].Ports).To(And(
							HaveLen(1),
							Satisfy(func(p corev1.ContainerPort) bool {
								return p.Name == "ssh" && p.ContainerPort == int32(2222)
							}),
						))
					})
				})

				It("creates the service", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "services", "aws-host-arm64",
							"-n", namespace,
							"-o", "json",
						)
						js, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						s := corev1.Service{}
						Expect(json.Unmarshal([]byte(js), &s)).To(Succeed())
						Expect(s.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
						Expect(s.Spec.Selector).To(And(
							HaveLen(1),
							HaveKeyWithValue("may.konflux-ci.dev/host", "aws-host-arm64"),
						))
						Expect(s.Spec.Ports).To(And(
							HaveLen(1),
							Satisfy(func(p corev1.ServicePort) bool {
								return p.TargetPort == intstr.FromInt(2222) &&
									p.Protocol == corev1.ProtocolTCP &&
									p.Name == "ssh" &&
									p.Port == int32(22)
							}),
						))
					})
				})

				It("sets the Ready state", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "statichosts.may.konflux-ci.dev", "aws-host-arm64",
							"-n", namespace,
							"-o", "go-template="+
								"{{ .status.state }}"+
								"{{ \"\\n\" }}",
						)
						sh, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						ll := utils.GetNonEmptyLines(sh)
						g.Expect(ll).To(HaveLen(1))
						g.Expect(ll[0]).To(Equal(maykonfluxcidevv1alpha1.HostActualStateReady))
					})
				})

				AfterAll(func() {
					cmd := exec.Command("kubectl", "delete", "-f", "-")
					cmd.Stdin = strings.NewReader(staticHostAwsHostArm64)
					_, _ = utils.Run(cmd)
					cmd = exec.Command("kubectl", "delete", "namespace", namespace)
					_, _ = utils.Run(cmd)
				})
			})
		})

		var _ = Describe("DynamicHost", Ordered, func() {

			var namespace = "e2e-dynamic-host"

			When("a StaticHost is created", func() {
				BeforeAll(func() {
					cmd := exec.Command("kubectl", "create", "namespace", namespace)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("kubectl", "apply", "--namespace", namespace, "-f", "-")
					cmd.Stdin = strings.NewReader(staticHostAwsHostArm64)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())
				})

				It("sets the Pending state", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "dynamichosts.may.konflux-ci.dev", "aws-host-arm64",
							"-n", namespace,
							"-o", "go-template="+
								"{{ .status.state }}"+
								"{{ \"\\n\" }}",
						)
						sh, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						ll := utils.GetNonEmptyLines(sh)
						g.Expect(ll).To(HaveLen(1))
						g.Expect(ll[0]).To(Equal(maykonfluxcidevv1alpha1.HostActualStatePending))
					})
				})

				It("creates the secret", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "secrets", "ssh-aws-host-arm64-key",
							"-n", namespace,
							"-o", "json",
						)
						js, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						s := corev1.Secret{}
						Expect(json.Unmarshal([]byte(js), &s)).To(Succeed())
						Expect(s.Data).To(
							And(
								Not(BeEmpty()),
								HaveKey("id_rsa"),
								HaveKey("id_rsa.pub"),
							))
					})
				})

				It("creates the pod", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "pods", "aws-host-arm64",
							"-n", namespace,
							"-o", "json",
						)
						jp, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						p := corev1.Pod{}
						Expect(json.Unmarshal([]byte(jp), &p)).To(Succeed())
						Expect(p.Labels).To(And(
							Not(BeEmpty()),
							HaveKeyWithValue("may.konflux-ci.dev/host", "aws-host-arm64"),
						))
						Expect(p.Spec.Containers).To(HaveLen(1))
						Expect(p.Spec.Containers[0].Env).To(
							And(
								Satisfy(func(e corev1.EnvVar) bool {
									return e.Name == "PUBLIC_KEY" &&
										e.ValueFrom != nil &&
										e.ValueFrom.SecretKeyRef == nil &&
										e.ValueFrom.SecretKeyRef.Key == "id_rsa.pub" &&
										e.ValueFrom.SecretKeyRef.LocalObjectReference.Name == "ssh-aws-host-arm64-key"
								}),
								Satisfy(func(e corev1.EnvVar) bool {
									return e.Name == "USER_NAME" && e.Value == "admin"
								}),
							))
						Expect(p.Spec.Containers[0].Ports).To(And(
							HaveLen(1),
							Satisfy(func(p corev1.ContainerPort) bool {
								return p.Name == "ssh" && p.ContainerPort == int32(2222)
							}),
						))
					})
				})

				It("creates the service", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "services", "aws-host-arm64",
							"-n", namespace,
							"-o", "json",
						)
						js, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						s := corev1.Service{}
						Expect(json.Unmarshal([]byte(js), &s)).To(Succeed())
						Expect(s.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
						Expect(s.Spec.Selector).To(And(
							HaveLen(1),
							HaveKeyWithValue("may.konflux-ci.dev/host", "aws-host-arm64"),
						))
						Expect(s.Spec.Ports).To(And(
							HaveLen(1),
							Satisfy(func(p corev1.ServicePort) bool {
								return p.TargetPort == intstr.FromInt(2222) &&
									p.Protocol == corev1.ProtocolTCP &&
									p.Name == "ssh" &&
									p.Port == int32(22)
							}),
						))
					})
				})

				It("sets the Ready state", func(ctx context.Context) {
					Eventually(func(g Gomega) {
						cmd := exec.Command(
							"kubectl", "get", "statichosts.may.konflux-ci.dev", "aws-host-arm64",
							"-n", namespace,
							"-o", "go-template="+
								"{{ .status.state }}"+
								"{{ \"\\n\" }}",
						)
						sh, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						ll := utils.GetNonEmptyLines(sh)
						g.Expect(ll).To(HaveLen(1))
						g.Expect(ll[0]).To(Equal(maykonfluxcidevv1alpha1.HostActualStateReady))
					})
				})

				AfterAll(func() {
					cmd := exec.Command("kubectl", "delete", "-f", "-")
					cmd.Stdin = strings.NewReader(staticHostAwsHostArm64)
					_, _ = utils.Run(cmd)
					cmd = exec.Command("kubectl", "delete", "namespace", namespace)
					_, _ = utils.Run(cmd)
				})
			})
		})
	})
})

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

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
