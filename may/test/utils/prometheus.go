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
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func IsPrometheusInstalled() bool {
	prometheusCRDS := []string{
		"alertmanagerconfigs.monitoring.coreos.com",
		"alertmanagers.monitoring.coreos.com",
		"podmonitors.monitoring.coreos.com",
		"probes.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"scrapeconfigs.monitoring.coreos.com",
		"servicemonitors.monitoring.coreos.com",
		"thanosrulers.monitoring.coreos.com",
	}
	return CheckCRDs(prometheusCRDS)
}

func InstallPrometheus() error {
	rootDir, err := GetProjectDir()
	if err != nil {
		return err
	}
	prometheusDir, err := filepath.Abs(filepath.Join(rootDir, "..", "demo", "dependencies", "prometheus"))
	if err != nil {
		return fmt.Errorf("failed to canonicalize prometheus configuration directory: %w", err)
	}

	// install crds before main prometheus controllers
	subdirs := []string{"crds", "controller"}

	for _, subdir := range subdirs {
		dir := filepath.Join(prometheusDir, subdir)
		// we need to explicitly invoke kustomize here since kubectl doesn't inflate helm charts
		cmd := exec.Command("kustomize", "build", "--enable-helm", dir)
		manifests, err := Run(cmd)
		if err != nil {
			return fmt.Errorf("failed to render prometheus manifests: %w", err)
		}

		cmd = exec.Command("kubectl", "apply", "--server-side", "-f", "-")
		cmd.Stdin = strings.NewReader(manifests)
		_, err = Run(cmd)
		if err != nil {
			return fmt.Errorf("failed to apply prometheus manifests: %w", err)
		}
	}

	return nil
}

func UninstallPrometheus() error {
	rootDir, err := GetProjectDir()
	if err != nil {
		return err
	}
	prometheusDir, err := filepath.Abs(filepath.Join(rootDir, "..", "demo", "dependencies", "prometheus"))
	if err != nil {
		return fmt.Errorf("failed to canonicalize prometheus configuration directory: %w", err)
	}

	// install crds before main prometheus controllers
	subdirs := []string{"controller", "crds"}
	for _, subdir := range subdirs {
		dir := filepath.Join(prometheusDir, subdir)
		// we need to explicitly invoke kustomize here since kubectl doesn't inflate helm charts
		cmd := exec.Command("kustomize", "build", "--enable-helm", dir)
		manifests, err := Run(cmd)
		if err != nil {
			return fmt.Errorf("failed to render prometheus manifests: %w", err)
		}

		cmd = exec.Command("kubectl", "delete", "--ignore-not-found", "-f", "-")
		cmd.Stdin = strings.NewReader(manifests)
		_, err = Run(cmd)
		if err != nil {
			return fmt.Errorf("failed to apply prometheus manifests: %w", err)
		}
	}

	return nil
}
