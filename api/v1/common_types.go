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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReconcileStatus holds reconcile metadata and external resource status.
// Embed in resource status types to expose last reconcile time, errors, and provider status.
type ReconcileStatus struct {
	// ObservedGeneration is the .metadata.generation last reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastReconcileTime is when the controller last reconciled this resource.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// LastReconcileError is the last error message from reconcile, if any. Cleared on success.
	// +optional
	LastReconcileError string `json:"lastReconcileError,omitempty"`

	// ResourceStatus is the status of the resource in Thalassa (e.g. ready, pending).
	// +optional
	ResourceStatus string `json:"resourceStatus,omitempty"`
}

// SubnetRef references a subnet by Thalassa identity. When empty, the controller uses the manager's default-subnet-id
// (for the cluster). Subnet is resolved from the Thalassa API based on identity; no Kubernetes Subnet CR is used.
type SubnetRef struct {
	// Identity is the Thalassa subnet identity. If set, the controller uses this subnet. If empty, the manager's default-subnet-id is used.
	// +optional
	Identity string `json:"identity,omitempty"`
	// Selector is a label selector to select the subnet by labels from the Thalassa API. If multiple subnets match the selector, it will error.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// SubnetRef references a subnet by name and namespace. If set, the controller uses this subnet. If empty, the manager's default-subnet-id is used.
	// Can be used in combination with the DBaaS Manager API (subnets.iaas.controllers.thalassa.cloud/v1) to find the subnet
	// +optional
	SubnetRef *corev1.TypedObjectReference `json:"subnetRef,omitempty"`
}

// SecurityGroupRef references a security group by Thalassa identity, label selector, or an IaaS SecurityGroup CR (TypedObjectReference).
// When multiple fields are set, identity takes precedence, then selector, then securityGroupRef.
type SecurityGroupRef struct {
	// Identity is the Thalassa security group identity.
	// +optional
	Identity string `json:"identity,omitempty"`
	// Selector is a label selector to select the security group by labels from the Thalassa API. If multiple security groups match the selector, it will error.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
	// SecurityGroupRef references a security group by name and namespace. If set, the controller uses this security group. If empty, the manager's default-security-group-id is used.
	// Can be used in combination with the DBaaS Manager API (securitygroups.iaas.controllers.thalassa.cloud/v1) to find the security group
	// +optional
	SecurityGroupRef *corev1.TypedObjectReference `json:"securityGroupRef,omitempty"`
}

// ResourceMetadata allows optional name and label overrides for the created/updated resource in Thalassa.
// When set, these values are applied to the resource in Thalassa instead of defaulting from the Kubernetes object.
type ResourceMetadata struct {
	// Name overrides the name of the resource in Thalassa. If not set, the Kubernetes object name is used.
	// +optional
	Name *string `json:"name,omitempty"`

	// Labels are applied to the resource in Thalassa.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are applied to the resource in Thalassa.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}
