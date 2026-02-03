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

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
)

// DynamicHostReconciler reconciles a DynamicHost object
type DynamicHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=dynamichosts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DynamicHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("host", req)

	h := maykonfluxcidevv1alpha1.DynamicHost{}
	if err := r.Get(ctx, req.NamespacedName, &h); err != nil {
		if kerrors.IsNotFound(err) {
			l.Info("host not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// finalize if required
	if !h.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, &h)
	}
	// ensure finalizer is there
	if controllerutil.AddFinalizer(&h, InClusterDriverFinalizer) {
		return ctrl.Result{}, r.Update(ctx, &h)
	}

	// initialize state
	if h.Status.State == nil {
		l.Info("initializing host's state: Pending")
		h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStatePending)
		return ctrl.Result{}, r.Status().Update(ctx, &h)
	}

	switch h.Spec.Status {
	case maykonfluxcidevv1alpha1.HostStatusPending:
		return r.ensurePending(ctx, &h)
	case maykonfluxcidevv1alpha1.HostStatusReady:
		return r.ensureReady(ctx, &h)
	default:
		l.Info("requested status not implemented", "requested-state", h.Spec.Status)
		return ctrl.Result{}, nil
	}
}

func (r *DynamicHostReconciler) ensurePending(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) (ctrl.Result, error) {
	l := logf.FromContext(ctx)
	switch *h.Status.State {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		l.Info("host is already in Pending state: doing nothing")
		// no-op
		return ctrl.Result{}, nil
	default:
		l.Info("host can not be moved back to Pending state", "requested-state", h.Spec.Status)
		return ctrl.Result{}, nil
	}
}

func (r *DynamicHostReconciler) ensureReady(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) (ctrl.Result, error) {
	l := logf.FromContext(ctx).WithValues("state", h.Status.State)
	switch *h.Status.State {
	case maykonfluxcidevv1alpha1.HostActualStatePending:
		l.Info("ensuring resources exists")
		if err := r.ensureResourcesExists(ctx, h); err != nil {
			return ctrl.Result{}, err
		}
		l.Info("resources exists, updating status to Ready")
		h.Status.State = ptr.To(maykonfluxcidevv1alpha1.HostActualStateReady)
		return ctrl.Result{}, r.Status().Update(ctx, h)
	case maykonfluxcidevv1alpha1.HostActualStateReady:
		l.Info("ensuring resources exists")
		return ctrl.Result{}, r.ensureResourcesExists(ctx, h)
	default:
		l.Info("status not implemented", "actual-state", *h.Status.State)
		return ctrl.Result{}, nil
	}
}

func (r *DynamicHostReconciler) ensureResourcesExists(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	l := logf.FromContext(ctx)

	// ensure secret exists
	l.Info("ensuring Secret with RSA key exists")
	if err := r.ensureKeySecretExists(ctx, h); err != nil {
		l.Error(err, "error creating Secret with RSA key")
		return err
	}

	// ensure pod exists
	l.Info("ensuring Pod exists")
	if err := r.ensurePodExists(ctx, h); err != nil {
		l.Error(err, "error creating Pod")
		return err
	}

	// ensure service exists
	l.Info("ensuring Service exists")
	if err := r.ensureServiceExists(ctx, h); err != nil {
		l.Error(err, "error creating Service")
		return err
	}

	l.Info("resources created (Pod, Secret, Service)")
	return nil
}

func (r *DynamicHostReconciler) ensureKeySecretExists(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	s := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("ssh-%s-key", h.Name),
			Namespace: h.Namespace,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(&s), &s); err != nil {
		if kerrors.IsNotFound(err) {
			if err := controllerutil.SetControllerReference(h, &s, r.Scheme); err != nil {
				return err
			}

			pr, pu, err := r.generatePEMKey(ctx)
			if err != nil {
				return err
			}

			s.Labels = map[string]string{}
			s.Data = map[string][]byte{
				"id_rsa":     pr,
				"id_rsa.pub": pu,
			}
			return r.Create(ctx, &s)
		}
		return err
	}

	return nil
}

func (r *DynamicHostReconciler) generatePEMKey(ctx context.Context) ([]byte, []byte, error) {
	l := logf.FromContext(ctx)

	// Generate the private key
	privateKey, err := rsa.GenerateKey(rand.Reader, KeySize)
	if err != nil {
		l.Error(err, "Error generating RSA key")
		return nil, nil, err
	}

	// Convert the private key to PKCS#1 ASN.1 DER format
	privateKeyDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// Create a PEM block
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	// Get the public key from the private key
	publicSSHRsaKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	// Convert the public key to SSH Authorized Key format
	publicKeySSHAuthorized := ssh.MarshalAuthorizedKey(publicSSHRsaKey)

	return privateKeyPEM, publicKeySSHAuthorized, nil
}

func (r *DynamicHostReconciler) ensurePodExists(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(&p), &p); err != nil {
		if kerrors.IsNotFound(err) {
			p.Labels = map[string]string{
				"may.konflux-ci.dev/host":        h.Name,
				"may.konflux-ci.dev/runner-type": "dynamic",
			}
			p.Spec = corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:            "openssh-server-container",
						Image:           "linuxserver/openssh-server:version-10.0_p1-r9",
						ImagePullPolicy: "IfNotPresent",
						Env: []corev1.EnvVar{
							{
								Name: "PUBLIC_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										Key: "id_rsa.pub",
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "ssh-" + h.Name + "-key",
										},
									},
								},
							},
							{
								Name:  "SUDO_ACCESS",
								Value: "true",
							},
							{
								Name:  "USER_NAME",
								Value: "admin",
							},
							{
								Name:  "LOG_STDOUT",
								Value: "true",
							},
						},
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: int32(2222),
								Name:          "ssh",
							},
						},
					},
				},
			}
			return client.IgnoreAlreadyExists(r.Create(ctx, &p))
		}
		return err
	}

	return nil
}

func (r *DynamicHostReconciler) ensureServiceExists(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	p := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}

	if err := r.Get(ctx, client.ObjectKeyFromObject(&p), &p); err != nil {
		if kerrors.IsNotFound(err) {
			p.Spec = corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{
					{
						TargetPort: intstr.FromInt(2222),
						Protocol:   corev1.ProtocolTCP,
						Name:       "ssh",
						Port:       int32(22),
					},
				},
				Selector: map[string]string{
					"may.konflux-ci.dev/host": h.Name,
				},
			}
			return client.IgnoreAlreadyExists(r.Create(ctx, &p))
		}
		return err
	}

	return nil
}

func (r *DynamicHostReconciler) finalize(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) (ctrl.Result, error) {
	// this component needs to be the last one to finalize
	if len(h.Finalizers) > 1 {
		return ctrl.Result{}, nil
	}

	if err := errors.Join(
		r.ensurePodDoesNotExist(ctx, h),
		r.ensureSecretDoesNotExist(ctx, h),
		r.ensureServiceDoesNotExist(ctx, h),
	); err != nil {
		return ctrl.Result{}, err
	}

	if controllerutil.RemoveFinalizer(h, InClusterDriverFinalizer) {
		return ctrl.Result{}, r.Update(ctx, h)
	}
	return ctrl.Result{}, nil
}

func (r *DynamicHostReconciler) ensurePodDoesNotExist(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}
	return r.deleteResource(ctx, &p)
}

func (r *DynamicHostReconciler) ensureSecretDoesNotExist(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	s := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ssh-" + h.Name + "-key",
			Namespace: h.Namespace,
		},
	}
	return r.deleteResource(ctx, &s)
}

func (r *DynamicHostReconciler) ensureServiceDoesNotExist(ctx context.Context, h *maykonfluxcidevv1alpha1.DynamicHost) error {
	s := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.Name,
			Namespace: h.Namespace,
		},
	}
	return r.deleteResource(ctx, &s)
}

func (r *DynamicHostReconciler) deleteResource(ctx context.Context, obj client.Object) error {
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return client.IgnoreNotFound(err)
	}
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.DynamicHost{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return labels.
				SelectorFromSet(labels.Set{"may.konflux-ci.dev/driver": "incluster"}).
				Matches(labels.Set(object.GetLabels()))
		}))).
		Named("dynamichost-incluster").
		Complete(r)
}
