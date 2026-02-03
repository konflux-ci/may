package runner

import (
	"slices"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeReady string = "Ready"

	ConditionReasonReady string = "Ready"

	ConditionReasonInitializing   string = "Initializing"
	ConditionReasonCleaning       string = "Cleaning"
	ConditionReasonStopped        string = "Stopped"
	ConditionReasonFailed         string = "Failed"
	ConditionReasonCleaningFailed string = "CleaningFailed"

	FieldStatusReady   string = "status.ready"
	FieldIsStatusReady string = "status.is-ready"

	FieldSpecInUseBy                 string = "spec.inUseBy"
	FieldSpecInUseByValueReserved    string = "in-use"
	FieldSpecInUseByValueNotReserved string = "not-in-use"

	FieldSpecInUseByName      string = "spec.inUseBy.name"
	FieldSpecInUseByNamespace string = "spec.inUseBy.namespace"
)

func StatusConditionReadyReason(runner maykonfluxcidevv1alpha1.Runner) string {
	c := apimeta.FindStatusCondition(runner.Status.Conditions, ConditionTypeReady)
	if c == nil {
		return ""
	}
	return c.Reason
}

func IsReadySet(runner maykonfluxcidevv1alpha1.Runner) bool {
	return apimeta.FindStatusCondition(runner.Status.Conditions, ConditionTypeReady) != nil
}

func IsReady(runner maykonfluxcidevv1alpha1.Runner) bool {
	return apimeta.IsStatusConditionTrue(runner.Status.Conditions, ConditionTypeReady)
}

func IsNotReadyWithReason(runner maykonfluxcidevv1alpha1.Runner, reason string) bool {
	c := apimeta.FindStatusCondition(runner.Status.Conditions, ConditionTypeReady)
	return c != nil && c.Status == metav1.ConditionFalse && c.Reason == reason
}

func IsCleaning(runner maykonfluxcidevv1alpha1.Runner) bool {
	return IsNotReadyWithReason(runner, ConditionReasonCleaning)
}

func IsStopped(runner maykonfluxcidevv1alpha1.Runner) bool {
	return IsNotReadyWithReason(runner, ConditionReasonStopped)
}

func IsInitializing(runner maykonfluxcidevv1alpha1.Runner) bool {
	return IsNotReadyWithReason(runner, ConditionReasonInitializing)
}

func IsFailed(runner maykonfluxcidevv1alpha1.Runner) bool {
	return IsNotReadyWithReason(runner, ConditionReasonFailed)
}

func SetReady(runner *maykonfluxcidevv1alpha1.Runner) bool {
	return apimeta.SetStatusCondition(&runner.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionReasonReady,
		Message: "Ready",
	})
}

func SetNotReady(runner *maykonfluxcidevv1alpha1.Runner, reason, message string) bool {
	return apimeta.SetStatusCondition(&runner.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

func SetNotReadyFailed(runner *maykonfluxcidevv1alpha1.Runner, message string) bool {
	return apimeta.SetStatusCondition(&runner.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonFailed,
		Message: message,
	})
}

func SetNotReadyCleaning(runner *maykonfluxcidevv1alpha1.Runner) bool {
	return apimeta.SetStatusCondition(&runner.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonCleaning,
		Message: "Cleaning",
	})
}

func SetNotReadyCleaningFailed(runner *maykonfluxcidevv1alpha1.Runner, message string) bool {
	return apimeta.SetStatusCondition(&runner.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonCleaningFailed,
		Message: message,
	})
}

func SetNotReadyInitializing(runner *maykonfluxcidevv1alpha1.Runner) bool {
	return apimeta.SetStatusCondition(&runner.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonInitializing,
		Message: "Initializing",
	})
}

func IsReserved(r maykonfluxcidevv1alpha1.Runner) bool {
	return r.Spec.InUseBy != nil &&
		r.Spec.InUseBy.Name != "" &&
		r.Spec.InUseBy.Namespace != ""
}

func IsInUseBy(r maykonfluxcidevv1alpha1.Runner, c maykonfluxcidevv1alpha1.Claim) bool {
	return r.Spec.InUseBy != nil &&
		r.Spec.InUseBy.Name == c.Name &&
		r.Spec.InUseBy.Namespace == c.Namespace
}

func SetInUseBy(runner *maykonfluxcidevv1alpha1.Runner, claim maykonfluxcidevv1alpha1.Claim) {
	// propagate pipeline name label
	if runner.Labels == nil {
		runner.Labels = map[string]string{}
	}
	runner.Labels["tekton.dev/pipeline"] = claim.Labels["tekton.dev/pipeline"]

	// set inUseBy
	runner.Spec.InUseBy = &maykonfluxcidevv1alpha1.ClaimReference{
		Name:      claim.Name,
		Namespace: claim.Namespace,
	}
}

func IndexHookStatus(hooksStatus []maykonfluxcidevv1alpha1.RunnerHookStatus, name string) int {
	return slices.IndexFunc(hooksStatus,
		func(s maykonfluxcidevv1alpha1.RunnerHookStatus) bool { return s.Hook == name })
}

func FindHookStatus(
	hooksStatus []maykonfluxcidevv1alpha1.RunnerHookStatus,
	name string,
) *maykonfluxcidevv1alpha1.RunnerHookStatus {
	if ix := IndexHookStatus(hooksStatus, name); ix != -1 {
		return &hooksStatus[ix]
	}
	return nil
}
