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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgresGrantSpec defines the desired state of a PostgreSQL grant on a Thalassa DB cluster.
type PostgresGrantSpec struct {
	// ClusterRef references the PostgresCluster (or cluster by identity) that this grant belongs to.
	// +kubebuilder:validation:Required
	ClusterRef PostgresClusterRef `json:"clusterRef"`

	// Name is the human-readable name of the grant (provider resource name).
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// RoleName is the PostgreSQL role name to grant privileges to.
	// +kubebuilder:validation:Required
	RoleName string `json:"roleName"`

	// DatabaseName is the PostgreSQL database name to grant privileges on.
	// +kubebuilder:validation:Required
	DatabaseName string `json:"databaseName"`

	// Read allows reading from the database.
	// +optional
	Read bool `json:"read,omitempty"`

	// Write allows writing to the database.
	// +optional
	Write bool `json:"write,omitempty"`
}

// PostgresGrantStatus defines the observed state of PostgresGrant.
type PostgresGrantStatus struct {
	ReconcileStatus `json:",inline"`

	// ResourceID is the Thalassa identity of the PostgreSQL grant.
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// Conditions represent the current state of the PostgresGrant.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterRef.name`
// +kubebuilder:printcolumn:name="Resource ID",type=string,JSONPath=`.status.resourceId`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Role",priority=1,type=string,JSONPath=`.spec.roleName`
// +kubebuilder:printcolumn:name="Database",priority=1,type=string,JSONPath=`.spec.databaseName`
// +kubebuilder:printcolumn:name="Read",priority=1,type=boolean,JSONPath=`.spec.read`
// +kubebuilder:printcolumn:name="Write",priority=1,type=boolean,JSONPath=`.spec.write`

// PostgresGrant is the Schema for the PostgreSQL grant API (granting privileges to a role on a database).
type PostgresGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresGrantSpec   `json:"spec"`
	Status PostgresGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresGrantList contains a list of PostgresGrant
type PostgresGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresGrant `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &PostgresGrant{}, &PostgresGrantList{})
}
