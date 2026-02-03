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

package provisioner

import (
	"context"
	"errors"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// DynamicHostGarbageCollector reconciles a Claim object
type DynamicHostGarbageCollector struct {
	client.Client
	Scheme *runtime.Scheme

	Namespace string
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DynamicHostGarbageCollector) Reconcile(
	ctx context.Context,
	_ ctrl.Request,
) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	l.Info("reconciling")

	hh, err := r.retrieveData(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(hh) == 0 {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, r.deleteDynamicHosts(ctx, hh)
}

func (r *DynamicHostGarbageCollector) deleteDynamicHosts(
	ctx context.Context,
	hosts []maykonfluxcidevv1alpha1.DynamicHost,
) error {
	// instantiate DynamicHost
	errs := []error{}
	for _, h := range hosts {
		errs = append(errs, r.Delete(ctx, &h))
	}

	// create host
	return errors.Join(errs...)
}

func (r *DynamicHostGarbageCollector) retrieveData(ctx context.Context) ([]maykonfluxcidevv1alpha1.DynamicHost, error) {
	// retrieve all DynamicHosts
	hh := maykonfluxcidevv1alpha1.DynamicHostList{}
	if err := r.List(ctx, &hh, client.InNamespace(r.Namespace)); err != nil {
		return nil, err
	}

	fhh := make([]maykonfluxcidevv1alpha1.DynamicHost, 0, len(hh.Items))
	for _, h := range hh.Items {
		if h.Status.State != nil && *h.Status.State == maykonfluxcidevv1alpha1.HostActualStateDrained {
			fhh = append(fhh, h)
		}
	}
	return fhh, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicHostGarbageCollector) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Claim{}).
		Watches(&maykonfluxcidevv1alpha1.DynamicHost{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []ctrl.Request {
				// we don't really care about the obj, we just want to be triggered
				return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
			}),
		).
		Named("dynamichost-gc").
		Complete(r)
}
