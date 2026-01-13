package status

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Set adds or updates a condition in the condition slice.
// It sets LastTransitionTime to the current time and ObservedGeneration to the provided generation.
func Set(conditions *[]metav1.Condition, generation int64, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
	})
}

// True sets a condition to True status.
func True(conditions *[]metav1.Condition, generation int64, conditionType, reason, message string) {
	Set(conditions, generation, conditionType, metav1.ConditionTrue, reason, message)
}

// False sets a condition to False status.
func False(conditions *[]metav1.Condition, generation int64, conditionType, reason, message string) {
	Set(conditions, generation, conditionType, metav1.ConditionFalse, reason, message)
}

// Unknown sets a condition to Unknown status.
func Unknown(conditions *[]metav1.Condition, generation int64, conditionType, reason, message string) {
	Set(conditions, generation, conditionType, metav1.ConditionUnknown, reason, message)
}

// Remove removes a condition from the slice.
func Remove(conditions *[]metav1.Condition, conditionType string) {
	meta.RemoveStatusCondition(conditions, conditionType)
}

// Get returns the condition with the given type, or nil if not found.
func Get(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(conditions, conditionType)
}

// IsTrue returns true if the condition with the given type has Status=True.
func IsTrue(conditions []metav1.Condition, conditionType string) bool {
	return meta.IsStatusConditionTrue(conditions, conditionType)
}

// IsFalse returns true if the condition with the given type has Status=False.
func IsFalse(conditions []metav1.Condition, conditionType string) bool {
	return meta.IsStatusConditionFalse(conditions, conditionType)
}
