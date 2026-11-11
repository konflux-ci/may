package provisioner

import (
	"github.com/go-logr/logr"
	"github.com/konflux-ci/may/internal/controller/provisioner/dynamichost"
	"github.com/konflux-ci/may/internal/controller/provisioner/runner"
	"github.com/konflux-ci/may/internal/controller/provisioner/runnerhook"
	"github.com/konflux-ci/may/internal/controller/provisioner/statichost"
	ctrl "sigs.k8s.io/controller-runtime"
)

type reconciler interface {
	SetupWithManager(mgr ctrl.Manager) error
}

func SetupWithManager(mgr ctrl.Manager, setupLog logr.Logger, namespace string) error {
	reconcilers := map[string]reconciler{
		"Runner":                          &runner.RunnerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"StaticHost":                      &statichost.StaticHostReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"RunnerHook":                      &runnerhook.RunnerHookReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"DynamicHostReconciler":           &dynamichost.DynamicHostReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"DynamicHostProvisioner":          &dynamichost.DynamicHostProvisioner{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Namespace: namespace},
		"DynamicHostGarbageCollector ":    &dynamichost.DynamicHostGarbageCollector{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Namespace: namespace},
		"DynamicHostAutoscalerReconciler": &dynamichost.DynamicHostAutoscalerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
	}

	for n, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", n)
			return err
		}
	}

	return nil
}
