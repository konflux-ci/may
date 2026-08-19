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

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	certmanagerVersion = "v1.18.2"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}

	// Delete leftover leases in kube-system (not cleaned by default)
	kubeSystemLeases := []string{
		"cert-manager-cainjector-leader-election",
		"cert-manager-controller",
	}
	for _, lease := range kubeSystemLeases {
		cmd = exec.Command("kubectl", "delete", "lease", lease,
			"-n", "kube-system", "--ignore-not-found", "--force", "--grace-period=0")
		if _, err := Run(cmd); err != nil {
			warnError(err)
		}
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// GetOTPKustomizationDir returns the absolute path to the OTP server kustomization
// (multi-platform-controller OTP from demo/dependencies). Relative to project dir (may/).
func GetOTPKustomizationDir() (string, error) {
	projectDir, err := GetProjectDir()
	if err != nil {
		return "", err
	}
	otpDir := filepath.Join(projectDir, "..", "demo", "dependencies", "multi-platform-controller", "config", "otp")
	abs, err := filepath.Abs(otpDir)
	if err != nil {
		return "", fmt.Errorf("resolve OTP kustomization dir: %w", err)
	}
	return abs, nil
}

// InstallOTPServer installs the OTP server from konflux-ci/multi-platform-controller
// using the kustomization at demo/dependencies/multi-platform-controller/config/otp.
// Requires cert-manager to be installed (for the OTP TLS certificate).
func InstallOTPServer() error {
	otpDir, err := GetOTPKustomizationDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("kubectl", "apply", "-k", otpDir)
	if _, err := Run(cmd); err != nil {
		return err
	}
	cmd = exec.Command("kubectl", "wait", "deployment.apps/multi-platform-otp-server",
		"--for", "condition=Available",
		"--namespace", "may-system",
		"--timeout", "5m",
	)
	_, err = Run(cmd)
	return err
}

// UninstallOTPServer removes the OTP server resources installed by InstallOTPServer.
func UninstallOTPServer() {
	otpDir, err := GetOTPKustomizationDir()
	if err != nil {
		warnError(err)
		return
	}
	cmd := exec.Command("kubectl", "delete", "-k", otpDir, "--ignore-not-found", "--timeout", "60s")
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// GetKueueCRDDir returns the absolute path to the Kueue CRD bases directory inside the
// sigs.k8s.io/kueue module (same version as go.mod). Used to install only the Kueue CRDs
// (e.g. ClusterQueue) in e2e, mirroring what the integration suite loads via envtest.
func GetKueueCRDDir() (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "sigs.k8s.io/kueue")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to locate sigs.k8s.io/kueue module: %w", err)
	}
	moduleDir := strings.TrimSpace(out.String())
	return filepath.Join(moduleDir, "config", "components", "crd", "bases"), nil
}

// InstallKueueCRDs installs the Kueue CRDs from the sigs.k8s.io/kueue module so the may
// controller can create and delete ClusterQueue objects in e2e. Only the CRDs are installed
// (no Kueue controller or webhooks), matching what the integration suite uses. Server-side
// apply avoids the client-side "annotations too long" error on Kueue's large CRDs.
func InstallKueueCRDs() error {
	crdDir, err := GetKueueCRDDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("kubectl", "apply", "--server-side", "-f", crdDir)
	_, err = Run(cmd)
	return err
}

// UninstallKueueCRDs removes the Kueue CRDs installed by InstallKueueCRDs.
func UninstallKueueCRDs() {
	crdDir, err := GetKueueCRDDir()
	if err != nil {
		warnError(err)
		return
	}
	cmd := exec.Command("kubectl", "delete", "-f", crdDir, "--ignore-not-found", "--timeout", "60s")
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsKueueCRDsInstalled checks whether the Kueue ClusterQueue CRD is present on the cluster.
func IsKueueCRDsInstalled() bool {
	return CheckCRDs([]string{"clusterqueues.kueue.x-k8s.io"})
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}
	return CheckCRDs(certManagerCRDs)
}

func CheckCRDs(crdNames []string) bool {
	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range crdNames {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// false positive
	// nolint:gosec
	if err = os.WriteFile(filename, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
