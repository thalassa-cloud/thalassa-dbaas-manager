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

// PostgresDatabaseExtension specifies a PostgreSQL extension to enable on the database.
type PostgresDatabaseExtension struct {
	// Name is the name of the extension (e.g. pg_trgm, uuid-ossp).
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// PostgresDatabaseSpec defines the desired state of a PostgreSQL database on a Thalassa DB cluster.
type PostgresDatabaseSpec struct {
	// ClusterRef references the PostgresCluster (or cluster by identity) that this database belongs to.
	// +kubebuilder:validation:Required
	ClusterRef PostgresClusterRef `json:"clusterRef"`

	// Name is the PostgreSQL database name (CREATE DATABASE name).
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Owner is the name of the PostgreSQL role that owns the database (OWNER attribute).
	// +kubebuilder:validation:Required
	Owner string `json:"owner"`

	// AllowConnections allows connections to the database. If false, no one can connect. Defaults to true.
	// +optional
	AllowConnections *bool `json:"allowConnections,omitempty"`

	// ConnectionLimit is the maximum number of concurrent connections to the database. -1 means no limit (PostgreSQL default).
	// +optional
	ConnectionLimit *int `json:"connectionLimit,omitempty"`

	// Extensions is a list of PostgreSQL extensions to enable on the database.
	// +optional
	Extensions []PostgresDatabaseExtension `json:"extensions,omitempty"`
}

// PostgresDatabaseStatus defines the observed state of PostgresDatabase.
type PostgresDatabaseStatus struct {
	ReconcileStatus `json:",inline"`

	// ResourceID is the Thalassa identity of the PostgreSQL database.
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// Conditions represent the current state of the PostgresDatabase.
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
// +kubebuilder:printcolumn:name="Owner",priority=1,type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Database Name",priority=1,type=string,JSONPath=`.spec.name`

// PostgresDatabase is the Schema for the PostgreSQL database API (database on a Thalassa DB cluster).
type PostgresDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresDatabaseSpec   `json:"spec"`
	Status PostgresDatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresDatabaseList contains a list of PostgresDatabase.
type PostgresDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresDatabase{}, &PostgresDatabaseList{})
}
