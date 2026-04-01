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

// PostgresClusterRef references a PostgresCluster by name or by Thalassa cluster identity.
type PostgresClusterRef struct {
	// Name is the name of the PostgresCluster resource in Kubernetes.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the PostgresCluster resource. Defaults to the same namespace as the referencing resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Identity is the Thalassa DB cluster identity. If set, the controller uses this directly instead of resolving from the PostgresCluster resource.
	// +optional
	Identity string `json:"identity,omitempty"`
}

// SecretKeySelector references a key in a Kubernetes Secret.
type SecretKeySelector struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key is the key in the Secret whose value will be used.
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Namespace is the namespace of the Secret. Defaults to the same namespace as the referencing resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SecretReference references a Secret by name (and optional namespace). Used when a controller creates/updates the Secret.
type SecretReference struct {
	// Name is the name of the Secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Namespace is the namespace of the Secret. Defaults to the same namespace as the referencing resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PostgresInitDbSpec defines optional PostgreSQL initdb settings for the initial database.
// Maps to CREATE DATABASE options (encoding, locale, etc.).
type PostgresInitDbSpec struct {
	// Encoding maps to ENCODING (character set). Cannot be changed after creation.
	// +optional
	Encoding string `json:"encoding,omitempty"`
	// Locale maps to LOCALE. Cannot be changed after creation.
	// +optional
	Locale string `json:"locale,omitempty"`
	// LocaleCollate maps to LC_COLLATE. Cannot be changed after creation.
	// +optional
	LocaleCollate string `json:"localeCollate,omitempty"`
	// LocaleCType maps to LC_CTYPE. Cannot be changed after creation.
	// +optional
	LocaleCType string `json:"localeCType,omitempty"`
	// DataChecksums enables data checksums (initdb --data-checksums).
	// +optional
	DataChecksums bool `json:"dataChecksums,omitempty"`
}

type DbInstanceTypeRef struct {
	// ID is the Thalassa instance type identity.
	// +kubebuilder:validation:Required
	ID string `json:"id"`
}

// PostgresClusterOnDeletePolicy defines how the Thalassa DB cluster is handled when the PostgresCluster resource is deleted.
// +kubebuilder:validation:Enum=Orphan;Delete;BackupAndDelete
type PostgresClusterOnDeletePolicy string

const (
	// PostgresClusterOnDeleteOrphan leaves the PostgreSQL cluster running in Thalassa; only the Kubernetes object is removed.
	PostgresClusterOnDeleteOrphan PostgresClusterOnDeletePolicy = "Orphan"
	// PostgresClusterOnDeleteDelete deletes the PostgreSQL cluster in Thalassa (default).
	PostgresClusterOnDeleteDelete PostgresClusterOnDeletePolicy = "Delete"
	// PostgresClusterOnDeleteBackupAndDelete triggers a final on-demand backup, waits for it to complete, then deletes the cluster.
	PostgresClusterOnDeleteBackupAndDelete PostgresClusterOnDeletePolicy = "BackupAndDelete"
)

// PostgresClusterSpec defines the desired state of a PostgreSQL DB cluster (Thalassa DBaaS).
type PostgresClusterSpec struct {
	// Metadata allows optional name and label overrides for the created resource in Thalassa.
	// +optional
	Metadata *ResourceMetadata `json:"metadata,omitempty"`

	// Description is an optional description of the cluster.
	// +optional
	Description string `json:"description,omitempty"`

	// +optional
	SubnetRef *SubnetRef `json:"subnet,omitempty"`

	// SecurityGroupRefs reference security groups to attach.
	// +optional
	SecurityGroupRefs []SecurityGroupRef `json:"securityGroups,omitempty"`

	// PostgresVersion is the PostgreSQL major version (e.g. "15", "16", "17") or full version (e.g. "16.10").
	// The controller resolves this to a Thalassa engine version identity.
	// +kubebuilder:validation:Required
	PostgresVersion string `json:"postgresVersion"`

	// InstanceType references the instance type to use for the cluster instances.
	// +kubebuilder:validation:Required
	InstanceType DbInstanceTypeRef `json:"instanceType"`

	// StorageGB is the allocated storage for each cluster instance in GB.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	StorageGB int32 `json:"storageGB"`

	// VolumeTypeClassId is the Thalassa volume type class ID for storage (e.g. SSD).
	// Required. Use the Thalassa IaaS API to list volume types.
	// +kubebuilder:validation:Required
	VolumeTypeClassId string `json:"volumeTypeClassId"`

	// Instances is the number of instances in the cluster
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=5
	// +optional
	Instances int32 `json:"instances,omitempty"`

	// InitDb specifies PostgreSQL initdb options.
	// +optional
	InitDb *PostgresInitDbSpec `json:"initDb,omitempty"`

	// Parameters is a map of PostgreSQL configuration parameters (e.g. shared_buffers, work_mem).
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`

	// MaintenanceDay is the day of the week for the maintenance window (0 = Sunday, 6 = Saturday).
	// +optional
	MaintenanceDay *int32 `json:"maintenanceDay,omitempty"`

	// MaintenanceStartAt is the start hour of the maintenance window in UTC (0–23).
	// +optional
	MaintenanceStartAt *int32 `json:"maintenanceStartAt,omitempty"`

	// DeleteProtection prevents the cluster from being deleted in Thalassa when true.
	// +optional
	DeleteProtection bool `json:"deleteProtection,omitempty"`

	// OnDelete defines how the Thalassa DB cluster is handled when the PostgresCluster resource is deleted.
	// Orphan leaves the cluster running in Thalassa. Delete removes the cluster in Thalassa (default).
	// BackupAndDelete takes a final on-demand backup, waits for it to complete, then deletes the cluster.
	// +kubebuilder:validation:Enum=Orphan;Delete;BackupAndDelete
	// +kubebuilder:default=Delete
	// +optional
	OnDelete PostgresClusterOnDeletePolicy `json:"onDelete,omitempty"`

	// ExposeService controls whether a Kubernetes Service is created to expose the cluster endpoint.
	// When true (default), a Service is created with port 5432 (read-write) and 5433 (read-only), pointing to the cluster.
	// +kubebuilder:default=true
	// +optional
	ExposeService *bool `json:"exposeService,omitempty"`

	// ServiceName is the name of the Service to create. Defaults to the PostgresCluster name.
	// Only used when ExposeService is true.
	// +optional
	ServiceName *string `json:"serviceName,omitempty"`

	// DbObjectStoreRef references the DbObjectStore to use for database backups.
	// +optional
	DbObjectStoreRef *DbObjectStoreRef `json:"dbObjectStore,omitempty"`

	// BackupSchedules is a list of backup schedules to create for the cluster.
	// +optional
	BackupSchedules []PostgresClusterBackupScheduleTemplateSpec `json:"backupSchedules,omitempty"`
}

type PostgresClusterBackupScheduleTemplateSpec struct {
	// Name is the name of the backup schedule in Thalassa.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Description is an optional description of the schedule.
	// +optional
	Description *string `json:"description,omitempty"`
	// Schedule is the cron expression for when to run backups (e.g. "0 2 * * *" for daily at 02:00).
	// +kubebuilder:validation:Required
	Schedule string `json:"schedule"`
}

type DbObjectStoreRef struct {
	// Identity is the Thalassa identity of the DbObjectStore. If set, the controller uses this directly instead of resolving from the DbObjectStore resource.
	// +optional
	Identity string `json:"identity,omitempty"`
	// Selector is a label selector to select the DbObjectStore by labels from the Thalassa API. If multiple DbObjectStore resources match the selector, it will error.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// DbObjectStoreRef references a DbObjectStore by name and namespace. If set, the controller uses this DbObjectStore. If empty, the manager's default-dbobjectstore-id is used.
	// Can be used in combination with the DbObjectStore API (dbobjectstores.dbaas.controllers.thalassa.cloud/v1) to find the DbObjectStore
	// +optional
	DbObjectStoreRef *corev1.TypedObjectReference `json:"dbObjectStoreRef,omitempty"`
}

// PostgresClusterStatus defines the observed state of PostgresCluster.
type PostgresClusterStatus struct {
	ReconcileStatus `json:",inline"`

	// ResourceID is the Thalassa identity of the DB cluster.
	// +optional
	ResourceID string `json:"resourceId,omitempty"`

	// EndpointHost is the host (IP or hostname) for connecting to the primary endpoint.
	// +optional
	EndpointHost string `json:"endpointHost,omitempty"`

	// Port is the port of the primary (read-write) endpoint (typically 5432).
	// +optional
	Port int32 `json:"port,omitempty"`

	// ReadOnlyEndpointHost is the host for the read-only endpoint, if available. When empty, use EndpointHost.
	// +optional
	ReadOnlyEndpointHost string `json:"readOnlyEndpointHost,omitempty"`

	// ReadOnlyPort is the port of the read-only endpoint (typically 5433). When 0, read replicas use the same port as Port.
	// +optional
	ReadOnlyPort int32 `json:"readOnlyPort,omitempty"`

	// EngineVersion is the resolved PostgreSQL engine version (e.g. "16.10").
	// +optional
	EngineVersion string `json:"engineVersion,omitempty"`

	// ReadyObservedAt is when the cluster was first observed with status "ready". Used to require the cluster
	// to remain ready for a minimum duration before setting the Ready condition to true.
	// +optional
	ReadyObservedAt *metav1.Time `json:"readyObservedAt,omitempty"`

	// Conditions represent the current state of the PostgresCluster.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InstanceCount is the number of instances in the cluster.
	// +optional
	InstanceCount int `json:"instanceCount,omitempty"`
	// Instances represent the current state of the cluster instances.
	// +optional
	Instances []PostgresClusterInstanceStatus `json:"instances,omitempty"`

	// ManagedBackupSchedules lists Thalassa backup schedules created by this controller from spec.backupSchedules (name and identity).
	// Only these schedules are deleted when removed from spec or when spec.backupSchedules is cleared; other API schedules are left untouched.
	// +listType=map
	// +listMapKey=name
	// +optional
	ManagedBackupSchedules []PostgresClusterManagedBackupSchedule `json:"managedBackupSchedules,omitempty"`
}

// PostgresClusterManagedBackupSchedule records a backup schedule the PostgresCluster controller created in Thalassa.
type PostgresClusterManagedBackupSchedule struct {
	// Name is the backup schedule name in Thalassa (matches spec.backupSchedules[].name).
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// Identity is the Thalassa identity of the schedule.
	// +kubebuilder:validation:Required
	Identity string `json:"identity"`
}

type PostgresClusterInstanceStatus struct {
	// Name is the name of the instance.
	// +optional
	Name string `json:"name,omitempty"`
	// IsPrimary is a flag to indicate if the instance is the primary instance.
	// +optional
	IsPrimary bool `json:"isPrimary,omitempty"`
	// IsPrimaryTarget is a flag to indicate if the instance is the target primary instance.
	// +optional
	IsPrimaryTarget bool `json:"isPrimaryTarget,omitempty"`
	// Replicating is a flag to indicate if the instance is replicating.
	// +optional
	Replicating bool `json:"replicating,omitempty"`
	// TimeLineID is the timeline ID of the instance.
	// +optional
	TimeLineID string `json:"timeLineID,omitempty"`
	// Healthy is a flag to indicate if the instance is healthy.
	// +optional
	Healthy bool `json:"healthy,omitempty"`
	// Joining is a flag to indicate if the instance is joining the cluster.
	// +optional
	Joining bool `json:"joining,omitempty"`
	// AvailabilityZone is the availability zone of the instance.
	// +optional
	AvailabilityZone string `json:"availabilityZone,omitempty"`
	// Version is the version of the instance.
	Version string `json:"version,omitempty"`
	// AllocatedStorage is the amount of storage allocated to the instance in GB.
	AllocatedStorage int `json:"allocatedStorage,omitempty"`
	// UsedStorage is the amount of storage used by the instance in GB.
	// May not always be available - feature gated functionality.
	UsedStorage int `json:"usedStorage,omitempty"`
	// Memory is the memory of the instance in MB.
	Memory int `json:"memory,omitempty"`
	// Cpu is the cpu of the instance in cores.
	Cpu int `json:"cpu,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.instances,statuspath=.status.instanceCount
// +kubebuilder:printcolumn:name="Resource ID",type=string,JSONPath=`.status.resourceId`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="endPoint",type=string,JSONPath=`.status.endpointHost`
// +kubebuilder:printcolumn:name="port",type=string,JSONPath=`.status.port`
// +kubebuilder:printcolumn:name="engineVersion",type=string,JSONPath=`.status.engineVersion`
// +kubebuilder:printcolumn:name="instances",type=integer,JSONPath=`.status.instanceCount`

// PostgresCluster is the Schema for the PostgreSQL DB cluster API (Thalassa DBaaS).
type PostgresCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresClusterSpec   `json:"spec"`
	Status PostgresClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresClusterList contains a list of PostgresCluster.
type PostgresClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresCluster{}, &PostgresClusterList{})
}
