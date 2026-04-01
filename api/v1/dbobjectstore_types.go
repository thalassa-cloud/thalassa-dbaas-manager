/*
Copyright 2026.

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

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbObjectStoreSpec defines the desired state of a Thalassa DB object store (backup storage).
type DbObjectStoreSpec struct {
	// Metadata allows optional name and label overrides for the resource in Thalassa.
	// +optional
	Metadata *ResourceMetadata `json:"metadata,omitempty"`

	// Description is an optional description of the object store.
	// +optional
	Description string `json:"description,omitempty"`

	// Region is the Thalassa cloud region identity or slug where the object store is created (immutable after create).
	// If empty, the manager uses --default-region / REGION, or discovers the region from the default subnet (see manager flags).
	// +optional
	Region string `json:"region,omitempty"`

	// RetentionPolicy is the retention policy for backups (e.g. "30d"). Omit for provider default.
	// +optional
	RetentionPolicy string `json:"retentionPolicy,omitempty"`

	// DeleteProtection prevents deletion of the object store in Thalassa when true.
	// +optional
	DeleteProtection bool `json:"deleteProtection,omitempty"`
}

// DbObjectStoreStatus defines the observed state of DbObjectStore.
type DbObjectStoreStatus struct {
	ReconcileStatus `json:",inline"`

	// ResourceID is the Thalassa identity of the object store.
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// StatusMessage is the provider status message, if any.
	// +optional
	StatusMessage string `json:"statusMessage,omitempty"`

	// Region is the Thalassa region identity of the object store.
	// +optional
	Region string `json:"region,omitempty"`

	// ServiceAccount is the Thalassa identity of the service account.
	// +optional
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// ObjectStorageBucket is the Thalassa identity of the object storage bucket.
	// +optional
	ObjectStorageBucket string `json:"objectStorageBucket,omitempty"`

	// ObjectStorageBucketUsage is the usage of the object storage bucket.
	// +optional
	ObjectStorageBucketUsage ObjectStorageBucketUsage `json:"objectStorageBucketUsage,omitempty"`

	// Conditions represent the current state of the DbObjectStore.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ObjectStorageBucketUsage struct {
	TotalSize    resource.Quantity `json:"totalSize"`
	TotalObjects int64             `json:"totalObjects"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.status.region`
// +kubebuilder:printcolumn:name="Resource ID",type=string,JSONPath=`.status.resourceId`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Total Size",type=string,JSONPath=`.status.objectStorageBucketUsage.totalSize`
// +kubebuilder:printcolumn:name="Total Objects",type=integer,JSONPath=`.status.objectStorageBucketUsage.totalObjects`
// +kubebuilder:printcolumn:name="Retention Policy",type=string,JSONPath=`.spec.retentionPolicy`

// DbObjectStore is the Schema for a Thalassa DB object store used for database backups (e.g. barman).
type DbObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DbObjectStoreSpec   `json:"spec"`
	Status DbObjectStoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DbObjectStoreList contains a list of DbObjectStore.
type DbObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DbObjectStore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DbObjectStore{}, &DbObjectStoreList{})
}
