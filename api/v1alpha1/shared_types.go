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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// BackupTarget describes a generic, cloud-agnostic object storage destination.
type BackupTarget struct {
	// Endpoint is the HTTP(S) endpoint for the object storage service.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Bucket is the bucket or container name.
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// Region is the AWS region to use for S3-compatible clients.
	// For AWS, this should match the bucket region (for example, "eu-west-1").
	// For many S3-compatible stores (MinIO/Ceph), this can be any non-empty value.
	// +optional
	// +kubebuilder:default=us-east-1
	Region string `json:"region,omitempty"`
	// PathPrefix is an optional prefix within the bucket for this cluster's snapshots.
	// +optional
	PathPrefix string `json:"pathPrefix,omitempty"`
	// RoleARN is the IAM role ARN (or S3-compatible equivalent) to assume via Web Identity.
	// When set, the backup Job mounts a projected ServiceAccount token and relies on the
	// cloud provider SDK default credential chain (for example, AWS IRSA).
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
	// CredentialsSecretRef optionally references a Secret containing credentials for the object store.
	// The Secret must exist in the same namespace as the OpenBaoCluster.
	// Cross-namespace references are not allowed for security reasons.
	// +optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
	// UsePathStyle controls whether to use path-style addressing (bucket.s3.amazonaws.com/object)
	// or virtual-hosted-style addressing (bucket.s3.amazonaws.com/object).
	// Set to true for MinIO and S3-compatible stores that require path-style.
	// Set to false for AWS S3 (default, as AWS is deprecating path-style).
	// +optional
	// +kubebuilder:default=false
	UsePathStyle bool `json:"usePathStyle,omitempty"`
	// PartSize is the size of each part in multipart uploads (in bytes).
	// Defaults to 10MB (10485760 bytes). Larger values may improve performance for large snapshots
	// on fast networks, while smaller values may be better for slow or unreliable networks.
	// +optional
	// +kubebuilder:default=10485760
	// +kubebuilder:validation:Minimum=5242880
	PartSize int64 `json:"partSize,omitempty"`
	// Concurrency is the number of concurrent parts to upload during multipart uploads.
	// Defaults to 3. Higher values may improve throughput on fast networks but increase
	// memory usage and may overwhelm slower storage backends.
	// +optional
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Concurrency int32 `json:"concurrency,omitempty"`
}
