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

package binder

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

// RunnerBinderReconciler reconciles a RunnerBinder object
type RunnerBinderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type OTPServerClient struct {
	*http.Client
}

func NewOTPServerClient(cert []byte) (*OTPServerClient, error) {
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(cert) {
		return nil, fmt.Errorf("error adding Cert to CertPool")
	}

	return &OTPServerClient{
		Client: NewTLSAuthClient(caCertPool),
	}, nil
}

func NewTLSAuthClient(certPool *x509.CertPool) *http.Client {
	// Setup HTTPS client
	tlsConfig := &tls.Config{
		RootCAs: certPool,
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	return &http.Client{Transport: transport}
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=may.konflux-ci.dev,resources=runners/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RunnerBinderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	l.Info("retrieving runner")
	// retrieve data
	u := maykonfluxcidevv1alpha1.Runner{}
	if err := r.Get(ctx, req.NamespacedName, &u); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l.Info("retrieving runner's secret")
	s := corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, &s); err != nil {
		return ctrl.Result{}, err
	}

	l.Info("retrieving claimer's secret")
	// ensure secret for TaskRun exists
	bs := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      u.Spec.InUseBy.Name,
			Namespace: u.Spec.InUseBy.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(&bs), &bs); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "error retrieving claimer's secret")
			return ctrl.Result{}, err
		}

		l.Info("claimer's secret does not exist, creating it")
		otpServerBaseUrl := fmt.Sprintf("https://multi-platform-otp-server.%s.svc.cluster.local", u.Namespace)

		l.Info("retrieving OTP Server tls cert secret")
		ots := corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "otp-tls-secrets", Namespace: u.Namespace}}
		if err := r.Get(ctx, client.ObjectKeyFromObject(&ots), &ots); err != nil {
			return ctrl.Result{}, err
		}
		cli, err := NewOTPServerClient(ots.Data["tls.crt"])
		if err != nil {
			return ctrl.Result{}, err
		}

		// send a post request to the OTPServer
		l.Info("creating secret in OTP Server")
		resp, err := cli.Post(otpServerBaseUrl+"/store-key", "text/plain", bytes.NewReader(s.Data["id_rsa"]))
		if err != nil {
			return ctrl.Result{}, err
		}

		l.Info("otp replied", "status-code", resp.StatusCode)
		rb, err := io.ReadAll(resp.Body)
		if err != nil {
			return ctrl.Result{}, err
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			l.Info("non-OK response from OTPServer", "status-code", resp.StatusCode, "body", rb)
			return ctrl.Result{}, fmt.Errorf("status code %v from OTPServer: %s", resp.StatusCode, rb)
		}

		// create secret for TaskRun
		bs := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      u.Spec.InUseBy.Name,
				Namespace: u.Spec.InUseBy.Namespace,
			},
			Data: map[string][]byte{
				"otp-ca":     ots.Data["tls.crt"],
				"otp":        rb,
				"otp-server": fmt.Appendf([]byte{}, "%s/otp", otpServerBaseUrl),
				"host":       fmt.Appendf([]byte{}, "%s@%s.%s.svc.cluster.local", string(u.UID), u.Name, u.Namespace),
				"user-dir":   fmt.Appendf([]byte{}, "/home/%s", u.Labels[constants.RunnerUserLabel]),
			},
			Type: corev1.SecretTypeOpaque,
		}
		l.Info("data persisted in OTP-Server, creating TaskRun's secret")
		if err := r.Create(ctx, &bs); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RunnerBinderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&maykonfluxcidevv1alpha1.Runner{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			u := object.(*maykonfluxcidevv1alpha1.Runner)
			return runner.IsReserved(*u)
		}))).
		Named("runnerbinder").
		Complete(r)
}
