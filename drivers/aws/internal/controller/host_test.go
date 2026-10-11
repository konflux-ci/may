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

package controller

import (
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func newTestScheme() *runtime.Scheme {
	GinkgoHelper()
	scheme := runtime.NewScheme()
	utilruntime.Must(maykonfluxcidevv1alpha1.AddToScheme(scheme))
	return scheme
}

func newTestStaticHost(name string, mutate func(*maykonfluxcidevv1alpha1.StaticHost)) *maykonfluxcidevv1alpha1.StaticHost {
	GinkgoHelper()
	host := &maykonfluxcidevv1alpha1.StaticHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				DriverLabel: DriverLabelValueAWS,
			},
		},
		Spec: maykonfluxcidevv1alpha1.StaticHostSpec{
			HostCoreSpec: maykonfluxcidevv1alpha1.HostCoreSpec{
				Flavor: "test-flavor",
				Status: maykonfluxcidevv1alpha1.HostStatusPending,
			},
			Runners: maykonfluxcidevv1alpha1.HostSpecRunners{
				Resources: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
				Instances: 1,
			},
		},
	}
	if mutate != nil {
		mutate(host)
	}
	return host
}

func newTestDynamicHost(name string, mutate func(*maykonfluxcidevv1alpha1.DynamicHost)) *maykonfluxcidevv1alpha1.DynamicHost {
	GinkgoHelper()
	host := &maykonfluxcidevv1alpha1.DynamicHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				DriverLabel: DriverLabelValueAWS,
			},
		},
		Spec: maykonfluxcidevv1alpha1.DynamicHostSpec{
			HostCoreSpec: maykonfluxcidevv1alpha1.HostCoreSpec{
				Flavor: "test-flavor",
				Status: maykonfluxcidevv1alpha1.HostStatusPending,
			},
			Runner: maykonfluxcidevv1alpha1.HostSpecRunner{
				Resources: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		},
	}
	if mutate != nil {
		mutate(host)
	}
	return host
}
