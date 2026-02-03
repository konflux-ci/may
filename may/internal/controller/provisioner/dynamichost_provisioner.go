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
	"fmt"
	"slices"
	"time"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const RootKeySecretName = "root-key"

var errDynamicHostAlreadyExist = fmt.Errorf("dynamic host already exist")

// DynamicHostProvisioner reconciles a Claim object
type DynamicHostProvisioner struct {
	client.Client
	Scheme *runtime.Scheme

	Namespace string
}

// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=claims,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DynamicHostProvisioner) Reconcile(
	ctx context.Context,
	_ ctrl.Request,
) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("reconciling")

	d, err := r.retrieveData(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.createNext(ctx, *d)
	if errors.Is(err, errDynamicHostAlreadyExist) {
		l.Info("DynamicHost already exists. Retrying in a little while to give caches time to sync")
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}
	return ctrl.Result{}, err
}

type data struct {
	claims       []maykonfluxcidevv1alpha1.Claim
	autoscalers  []maykonfluxcidevv1alpha1.DynamicHostAutoscaler
	dynamicHosts []maykonfluxcidevv1alpha1.DynamicHost
}

func (r *DynamicHostProvisioner) createNext(ctx context.Context, data data) error {
	l := log.FromContext(ctx)
	l.Info("creating a DynamicHost for the next Claim")

	// if no claims or no autoscalers, nothing we can do -we can cut short
	if len(data.claims) == 0 {
		l.Info("no claim to allocate")
		return nil
	}
	if len(data.autoscalers) == 0 {
		l.Info("no autoscalers registered")
		return nil
	}

	// sorting claims by CreationTimestamp
	slices.SortFunc(data.claims, func(c1, c2 maykonfluxcidevv1alpha1.Claim) int {
		return c1.CreationTimestamp.Compare(c2.CreationTimestamp.Time)
	})

	return r.processClaims(ctx, data)
}

func (r *DynamicHostProvisioner) processClaims(ctx context.Context, data data) error {
	for _, c := range data.claims {
		l := log.FromContext(ctx).
			WithValues("claim", c.Name, "flavor", c.Spec.Flavor)
		// check if DynamicHost already exists
		if slices.ContainsFunc(data.dynamicHosts, func(dh maykonfluxcidevv1alpha1.DynamicHost) bool {
			return dh.Name == c.Name
		}) {
			l.V(10).Info("A DynamicHost already exists")
			continue
		}

		// find an autoscaler
		if a := r.findAutoscalerForClaim(c, data.autoscalers); a != nil {
			return r.createDynamicHost(ctx, c, *a)
		}

		l.Info("autoscaler not found for claim's flavor")
	}
	return nil
}

func (r *DynamicHostProvisioner) findAutoscalerForClaim(
	c maykonfluxcidevv1alpha1.Claim,
	autoscalers []maykonfluxcidevv1alpha1.DynamicHostAutoscaler,
) *maykonfluxcidevv1alpha1.DynamicHostAutoscaler {
	for _, a := range autoscalers {
		if a.Spec.Flavor == c.Spec.Flavor {
			return &a
		}
	}
	return nil
}

func (r *DynamicHostProvisioner) createDynamicHost(
	ctx context.Context,
	c maykonfluxcidevv1alpha1.Claim,
	autoscaler maykonfluxcidevv1alpha1.DynamicHostAutoscaler,
) error {
	l := log.FromContext(ctx).WithValues("claim", c.Name)
	// instantiate DynamicHost
	host := maykonfluxcidevv1alpha1.DynamicHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name,
			Namespace: r.Namespace,
			Labels:    autoscaler.Labels,
		},
		Spec: autoscaler.Spec.Template.Spec,
	}

	// overwrite RootKey
	host.Spec.RootKey.Name = host.Name
	if host.Spec.Runner.Hooks != nil {
		overwriteRootKey(host, host.Spec.Runner.Hooks.Cleanup)
		overwriteRootKey(host, host.Spec.Runner.Hooks.Provisioning)
	}

	// create host
	if err := r.Create(ctx, &host); err != nil {
		if kerrors.IsAlreadyExists(err) {
			l.Info("already exist error while creating host for claim")
			return errors.Join(errDynamicHostAlreadyExist, err)
		}
		l.Info("error creating host for claim")
		return err
	}

	l.Info("host created",
		"host", host.Name,
		"flavor", host.Spec.Flavor,
		"namespace", host.Namespace)
	return nil
}

func overwriteRootKey(
	hr maykonfluxcidevv1alpha1.DynamicHost,
	tt []maykonfluxcidevv1alpha1.RunnerHookPodTemplateSpec,
) {
	for _, c := range tt {
		i := slices.IndexFunc(c.Template.Spec.Volumes, func(v corev1.Volume) bool {
			return v.Name == RootKeySecretName
		})
		if i < 0 {
			// root key not needed
			continue
		}

		v := corev1.Volume{
			Name: RootKeySecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  fmt.Sprintf("ssh-%s-key", hr.Name),
					Optional:    ptr.To(false),
					DefaultMode: ptr.To(int32(0o400)),
				},
			},
		}
		c.Template.Spec.Volumes[i] = v
	}
}

func (r *DynamicHostProvisioner) retrieveData(ctx context.Context) (*data, error) {
	// retrieve all Claims in Pending state
	cc := maykonfluxcidevv1alpha1.ClaimList{}
	if err := r.List(ctx, &cc,
		client.InNamespace(corev1.NamespaceAll),
		client.MatchingFields{
			claim.FieldStatusConditionClaimed: claim.ConditionReasonPending,
		}); err != nil {
		return nil, err
	}

	// retrieve all DynamichHostAutoscalers
	aa := maykonfluxcidevv1alpha1.DynamicHostAutoscalerList{}
	if err := r.List(ctx, &aa, client.InNamespace(r.Namespace)); err != nil {
		return nil, err
	}

	// retrieve all DynamicHosts
	hh := maykonfluxcidevv1alpha1.DynamicHostList{}
	if err := r.List(ctx, &hh, client.InNamespace(r.Namespace)); err != nil {
		return nil, err
	}

	return &data{
		claims:       cc.Items,
		autoscalers:  aa.Items,
		dynamicHosts: hh.Items,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicHostProvisioner) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Claim{}).
		Watches(&maykonfluxcidevv1alpha1.DynamicHostAutoscaler{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []ctrl.Request {
				// we don't really care about the obj, we just want to be triggered
				return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
			}),
		).
		Watches(&maykonfluxcidevv1alpha1.DynamicHost{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []ctrl.Request {
				// we don't really care about the obj, we just want to be triggered
				return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(obj)}}
			}),
		).
		Named("dynamichost-provisioner").
		Complete(r)
}
