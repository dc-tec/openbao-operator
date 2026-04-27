// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderedStorageSpec(rendered *RenderedExecutionContract) (openbaov1alpha1.StorageConfig, ValidationResult) {
	if rendered == nil || strings.TrimSpace(rendered.Storage.PrimarySize) == "" {
		return openbaov1alpha1.StorageConfig{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered execution contract must specify primary storage for same-cluster materialization.",
		}
	}

	return openbaov1alpha1.StorageConfig{
			Size:             rendered.Storage.PrimarySize,
			StorageClassName: cloneStringPtr(rendered.Storage.ClassName),
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered primary storage contract is compatible with OpenBaoCluster.",
		}
}

func renderedReadReplicas(rendered *RenderedExecutionContract) (*openbaov1alpha1.ReadReplicaConfig, ValidationResult) {
	if rendered == nil || rendered.Cluster.ReadReplicas <= 0 {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered contract does not require a read-replica pool.",
		}
	}

	readReplicas := &openbaov1alpha1.ReadReplicaConfig{
		Replicas: rendered.Cluster.ReadReplicas,
	}
	if rendered.Runtime.ReadReplica != nil {
		readReplicas.Template = cloneReadReplicaTemplateConfig(rendered.Runtime.ReadReplica.Template)
	}
	if rendered.Exposure.ReadReplicaServicePolicy != nil && rendered.Exposure.ReadReplicaServicePolicy.Enabled {
		readReplicas.Service = &openbaov1alpha1.ReadReplicaServiceConfig{
			Enabled:     true,
			Type:        corev1.ServiceType(rendered.Exposure.ReadReplicaServicePolicy.Type),
			Annotations: cloneStringMap(rendered.Exposure.ReadReplicaServicePolicy.Annotations),
		}
	}
	readReplicaSize := strings.TrimSpace(rendered.Storage.ReadReplicaSize)
	if readReplicaSize == "" && rendered.Storage.ReadReplicaClassName == nil {
		return readReplicas, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered read-replica pool uses default storage sizing.",
		}
	}

	readReplicas.Storage = &openbaov1alpha1.ReadReplicaStorageConfig{
		StorageClassName: cloneStringPtr(rendered.Storage.ReadReplicaClassName),
	}
	if readReplicaSize != "" {
		size, err := resource.ParseQuantity(readReplicaSize)
		if err != nil {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: fmt.Sprintf("Rendered read-replica storage size %q is not a valid Kubernetes quantity.", rendered.Storage.ReadReplicaSize),
			}
		}
		readReplicas.Storage.Size = &size
	}

	return readReplicas, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered read-replica contract is compatible with OpenBaoCluster.",
	}
}
