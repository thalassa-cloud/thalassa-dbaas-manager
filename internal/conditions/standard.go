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

package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceStatusReady is the value for ResourceStatus when the Thalassa resource is ready.
const ResourceStatusReady = "ready"

// ConditionState groups the high-level lifecycle states used with SetStandardConditions.
type ConditionState string

const (
	// ConditionStateAvailable means the resource is fully functional.
	ConditionStateAvailable ConditionState = "available"
	// ConditionStateDegraded means the resource failed to reach or maintain desired state.
	ConditionStateDegraded ConditionState = "degraded"
	// ConditionStateProgressing means the resource is being created or updated.
	ConditionStateProgressing ConditionState = "progressing"
)

// SetStandardConditions sets Ready and the standard condition types (Available, Progressing, Degraded)
// on the given conditions slice. state must be ConditionStateAvailable, ConditionStateDegraded, or ConditionStateProgressing.
func SetStandardConditions(conditions *[]metav1.Condition, state ConditionState, reason, message string) {
	switch state {
	case ConditionStateAvailable:
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Available", Status: metav1.ConditionTrue, Reason: reason, Message: message})
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Progressing", Status: metav1.ConditionFalse, Reason: "Reconciled", Message: ""})
		meta.RemoveStatusCondition(conditions, "Degraded")
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: reason, Message: message})
	case ConditionStateDegraded:
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Available", Status: metav1.ConditionFalse, Reason: reason, Message: message})
		meta.RemoveStatusCondition(conditions, "Progressing")
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Degraded", Status: metav1.ConditionTrue, Reason: reason, Message: message})
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: reason, Message: message})
	case ConditionStateProgressing:
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Available", Status: metav1.ConditionFalse, Reason: reason, Message: message})
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Progressing", Status: metav1.ConditionTrue, Reason: reason, Message: message})
		meta.RemoveStatusCondition(conditions, "Degraded")
		meta.SetStatusCondition(conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: reason, Message: message})
	}
}
