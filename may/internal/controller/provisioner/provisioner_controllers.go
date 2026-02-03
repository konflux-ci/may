package provisioner

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

type reconciler interface {
	SetupWithManager(mgr ctrl.Manager) error
}

func SetupWithManager(mgr ctrl.Manager, setupLog logr.Logger, namespace string) error {
	reconcilers := map[string]reconciler{
		"Runner":                          &RunnerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"StaticHost":                      &StaticHostReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"RunnerHook":                      &RunnerHookReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"DynamicHostReconciler":           &DynamicHostReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		"DynamicHostProvisioner":          &DynamicHostProvisioner{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Namespace: namespace},
		"DynamicHostGarbageCollector ":    &DynamicHostGarbageCollector{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Namespace: namespace},
		"DynamicHostAutoscalerReconciler": &DynamicHostAutoscalerReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
	}

	for n, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", n)
			return err
		}
	}

	return nil
}
