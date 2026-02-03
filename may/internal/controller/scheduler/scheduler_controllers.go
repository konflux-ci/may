package scheduler

import (
	"github.com/go-logr/logr"
	"github.com/konflux-ci/may/pkg/scheduler"
	ctrl "sigs.k8s.io/controller-runtime"
)

func SetupWithManager(mgr ctrl.Manager, setupLog logr.Logger, namespace string, sched scheduler.Scheduler) error {
	if err := (&ClaimReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Namespace: namespace,
		Scheduler: sched,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClaimReconciler")
		return err
	}

	return nil
}
