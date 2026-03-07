package workloadidentity

import (
	"maps"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// ServiceAccountAnnotations returns a safe copy of workload identity annotations.
func ServiceAccountAnnotations(target openbaov1alpha1.BackupTarget) map[string]string {
	if target.WorkloadIdentity == nil || len(target.WorkloadIdentity.ServiceAccountAnnotations) == 0 {
		return nil
	}

	return maps.Clone(target.WorkloadIdentity.ServiceAccountAnnotations)
}

// MergePodLabels appends workload identity labels without overriding operator-managed labels.
func MergePodLabels(labels map[string]string, target openbaov1alpha1.BackupTarget) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	if target.WorkloadIdentity == nil || len(target.WorkloadIdentity.PodLabels) == 0 {
		return labels
	}

	for key, value := range target.WorkloadIdentity.PodLabels {
		if _, exists := labels[key]; exists {
			continue
		}
		labels[key] = value
	}

	return labels
}
