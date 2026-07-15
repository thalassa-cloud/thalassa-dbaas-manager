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

// PostgresRoleSpec defines the desired state of a PostgreSQL role on a Thalassa DB cluster.
type PostgresRoleSpec struct {
	// ClusterRef references the PostgresCluster (or cluster by identity) that this role belongs to.
	// +kubebuilder:validation:Required
	ClusterRef PostgresClusterRef `json:"clusterRef"`

	// Name is the PostgreSQL role name (CREATE ROLE name).
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Login allows the role to log in (LOGIN attribute).
	// +kubebuilder:default=true
	// +optional
	Login bool `json:"login,omitempty"`

	// CreateDb allows the role to create databases (CREATEDB attribute).
	// +optional
	CreateDb bool `json:"createDb,omitempty"`

	// CreateRole allows the role to create other roles (CREATEROLE attribute).
	// +optional
	CreateRole bool `json:"createRole,omitempty"`

	// ConnectionLimit is the maximum number of concurrent connections for the role. -1 means no limit (PostgreSQL default).
	// +optional
	ConnectionLimit *int64 `json:"connectionLimit,omitempty"`

	// ValidUntil is the date and time the role will expire (VALID UNTIL). Omit for no expiry.
	// +optional
	ValidUntil *metav1.Time `json:"validUntil,omitempty"`

	// PasswordSecretRef references a Kubernetes Secret key containing the role password. If not set, password is not set or changed.
	// +optional
	PasswordSecretRef *SecretKeySelector `json:"passwordSecretRef,omitempty"`

	// WriteConnectionSecretToRef optionally creates or updates a Secret with connection details (host, port, username, password, database, connection string).
	// When set, the controller will create or update the referenced Secret with keys: username, password, host, port, database, uri (or connectionString).
	// If GeneratePassword is true, a random password is generated and used for the role; otherwise the password is read from PasswordSecretRef (if set).
	// +optional
	WriteConnectionSecretToRef *SecretReference `json:"writeConnectionSecretToRef,omitempty"`

	// GeneratePassword, when true and WriteConnectionSecretToRef is set, generates a random password for the role and writes it to the secret.
	// The generated password is used when creating or updating the role in Thalassa.
	// +kubebuilder:default=true
	// +optional
	GeneratePassword bool `json:"generatePassword,omitempty"`
}

// PostgresRoleStatus defines the observed state of PostgresRole.
type PostgresRoleStatus struct {
	ReconcileStatus `json:",inline"`

	// ResourceID is the Thalassa identity of the PostgreSQL role.
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// Conditions represent the current state of the PostgresRole.
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

// PostgresRole is the Schema for the PostgreSQL role API (role on a Thalassa DB cluster).
type PostgresRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresRoleSpec   `json:"spec"`
	Status PostgresRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresRoleList contains a list of PostgresRole.
type PostgresRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresRole `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &PostgresRole{}, &PostgresRoleList{})
}
