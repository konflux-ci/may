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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/test/utils"
)

// BinderContexts registers the RunnerBinder (Runner → Pod credentials) test contexts.
// When a Runner is bound (Claim scheduled to Runner), RunnerBinder creates a Secret
// in the Pod's namespace with OTP and host credentials; the OTP server must be installed.
func BinderContexts() {
	Context("RunnerBinder (Runner → Pod credentials)", Ordered, func() {
		BeforeAll(func() {
			By("creating tenant namespace for RunnerBinder tests")
			cmd := exec.Command("kubectl", "create", "namespace", binderTestNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("labeling the namespace as a tenant")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", binderTestNamespace,
				"konflux-ci.dev/type=tenant")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("deleting RunnerBinder test namespace")
			cmd := exec.Command("kubectl", "delete", "namespace", binderTestNamespace, "--ignore-not-found", "--wait=false")
			_, _ = utils.Run(cmd)
		})

		It("creates Secret with OTP and host when Runner is bound and Pod receives credentials", func() {
			runnerName := "runner-binder-secret"
			podName := "pod-binder-secret"
			applySpecificationWithStatus(runnerYAMLWithReadyStatusForBinder(runnerName, namespace, binderFlavor, binderRunnerUser))
			defer deleteRunner(runnerName)
			defer deletePod(binderTestNamespace, podName)
			defer deleteSecret(namespace, runnerName)

			By("creating Runner's Secret with id_rsa (required by RunnerBinder for OTP)")
			idRsaBase64 := base64.StdEncoding.EncodeToString([]byte("dummy-key-for-e2e"))
			applySpecification(runnerSecretYAML(runnerName, namespace, idRsaBase64))

			createPodWithFlavor(podName, binderTestNamespace, binderFlavor)

			By("verifying Claim gets Claimed and Runner gets spec.inUseBy")
			Eventually(func(g Gomega) {
				c := getClaim(g, binderTestNamespace, podName)
				g.Expect(claim.IsClaimed(*c)).To(BeTrue(), "expected Claim to be Claimed")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			var r *v1alpha1.Runner
			Eventually(func(g Gomega) {
				r = getRunner(g, namespace, runnerName)
				g.Expect(r.Spec.InUseBy).NotTo(BeNil())
				g.Expect(r.Spec.InUseBy.Name).To(Equal(podName))
				g.Expect(r.Spec.InUseBy.Namespace).To(Equal(binderTestNamespace))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying Secret exists in Pod namespace with OTP and host credentials")
			Eventually(func(g Gomega) {
				sec := getSecret(g, binderTestNamespace, podName)
				g.Expect(sec.Data).To(HaveKey("otp-ca"), "Secret should have otp-ca")
				g.Expect(sec.Data).To(HaveKey("otp"), "Secret should have otp (one-time password)")
				g.Expect(sec.Data).To(HaveKey("otp-server"), "Secret should have otp-server URL")
				g.Expect(sec.Data).To(HaveKey("host"), "Secret should have host")
				g.Expect(sec.Data).To(HaveKey("user-dir"), "Secret should have user-dir")

				host := string(r.UID) + "@" + runnerName + "." + namespace + ".svc.cluster.local"
				g.Expect(string(sec.Data["host"])).To(Equal(host),
					"host should be <runner-uid>@<runner>.<namespace>.svc.cluster.local")
				g.Expect(string(sec.Data["user-dir"])).To(Equal("/home/"+binderRunnerUser),
					"user-dir should be /home/<runner-user>")
			}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

			By("verifying Pod can retrieve the key from the OTP server using the credentials Secret")
			otpRetrieverPodName := podName + "-otp-retrieve"
			expectedKey := "dummy-key-for-e2e"
			applySpecification(otpRetrieverPodYAML(otpRetrieverPodName, podName, binderTestNamespace, expectedKey, ""))
			defer deletePod(binderTestNamespace, otpRetrieverPodName)
			waitForPodSucceeded(binderTestNamespace, otpRetrieverPodName)
		})
	})
}
