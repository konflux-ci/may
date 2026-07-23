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

package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/indexer"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	testEnv         *envtest.Environment
	cfg             *rest.Config
	k8sClient       client.Client
	k8sClientCache  cache.Cache
	k8sCachedClient client.Client
	k8sReader       client.Reader
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func(ctx context.Context) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	var err error

	Expect(maykonfluxcidevv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// setup direct API Server client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ShouldNot(HaveOccurred())
	k8sReader = k8sClient

	// setup cache with field indexers
	k8sClientCache, err = cache.New(cfg, cache.Options{Scheme: scheme.Scheme})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(indexer.SetupFieldIndexers(ctx, k8sClientCache, logr.Discard())).Should(Succeed())

	// setup the cached client
	k8sCachedClient, err = client.New(cfg, client.Options{
		Scheme: scheme.Scheme,
		Cache: &client.CacheOptions{
			Reader: k8sClientCache,
		},
	})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(k8sCachedClient).ShouldNot(BeNil())

	// derive a context that outlives BeforeSuite for the cache
	cacheCtx, cacheCancel := context.WithCancel(context.WithoutCancel(ctx))

	// start in background
	var wg sync.WaitGroup
	wg.Go(func() {
		defer GinkgoRecover()
		Expect(k8sClientCache.Start(cacheCtx)).Should(Or(
			Succeed(),
			MatchError(context.Canceled),
			MatchError(context.DeadlineExceeded),
		))
	})

	// wait for cache to sync
	Eventually(func(g Gomega) {
		g.Expect(k8sClientCache.WaitForCacheSync(ctx)).Should(BeTrue())
	}).WithTimeout(1 * time.Minute).Should(Succeed())

	DeferCleanup(func() {
		By("tearing down the test environment")
		cacheCancel()
		wg.Wait()
		Expect(testEnv.Stop()).NotTo(HaveOccurred())
	})
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
