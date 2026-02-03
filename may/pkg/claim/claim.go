package claim

import (
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeClaimed string = "Claimed"

	ConditionReasonScheduled string = "Scheduled"

	ConditionReasonPending     string = "Pending"
	ConditionReasonUnclaimable string = "Unclaimable"

	FieldStatusConditionClaimed string = "status.conditions[claimed]"
)

func IsClaimed(claim maykonfluxcidevv1alpha1.Claim) bool {
	return apimeta.IsStatusConditionTrue(claim.Status.Conditions, ConditionTypeClaimed)
}

func IsNotClaimedWithReason(claim maykonfluxcidevv1alpha1.Claim, reason string) bool {
	c := apimeta.FindStatusCondition(claim.Status.Conditions, ConditionTypeClaimed)
	return c != nil && c.Status == metav1.ConditionFalse && c.Reason == reason
}

func IsPending(claim maykonfluxcidevv1alpha1.Claim) bool {
	return IsNotClaimedWithReason(claim, ConditionReasonPending)
}

func IsUnclaimable(claim maykonfluxcidevv1alpha1.Claim) bool {
	return IsNotClaimedWithReason(claim, ConditionReasonUnclaimable)
}

func ClaimedConditionReason(claim maykonfluxcidevv1alpha1.Claim) string {
	c := apimeta.FindStatusCondition(claim.Status.Conditions, ConditionTypeClaimed)
	if c == nil {
		return ""
	}
	return c.Reason
}

func SetClaimed(claim *maykonfluxcidevv1alpha1.Claim) bool {
	return apimeta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeClaimed,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionReasonScheduled,
		Message: "Claimed",
	})
}

func SetToSchedule(claim *maykonfluxcidevv1alpha1.Claim) bool {
	return apimeta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeClaimed,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonPending,
		Message: "Waiting for a matching runner to become available",
	})
}

func SetNotClaimed(claim *maykonfluxcidevv1alpha1.Claim, reason, message string) bool {
	return apimeta.SetStatusCondition(&claim.Status.Conditions, metav1.Condition{
		Type:    ConditionTypeClaimed,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}
