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

package helpers

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

// RequeueAfterStatusUpdateFailure is the delay before requeuing when a status update fails.
const RequeueAfterStatusUpdateFailure = 15 * time.Second

// RequeueAfterDependencyNotReady is the delay when a referenced resource does not have ResourceID yet.
const RequeueAfterDependencyNotReady = 5 * time.Second

// ErrDependencyNotReady is returned by resolve helpers when the referenced resource exists but has no ResourceID yet. Callers should requeue instead of setting an error condition.
var ErrDependencyNotReady = errors.New("dependency does not have a resource ID yet")

// ResourceStatusReady is the value for ResourceStatus when the Thalassa resource is ready.
const ResourceStatusReady = stdconditions.ResourceStatusReady

// ConditionState groups the high-level lifecycle states used with SetStandardConditions.
type ConditionState = stdconditions.ConditionState

const (
	ConditionStateAvailable   = stdconditions.ConditionStateAvailable
	ConditionStateDegraded    = stdconditions.ConditionStateDegraded
	ConditionStateProgressing = stdconditions.ConditionStateProgressing
)

// SetStandardConditions sets Ready and the standard condition types (Available, Progressing, Degraded)
// on the given conditions slice. state must be ConditionStateAvailable, ConditionStateDegraded, or ConditionStateProgressing.
func SetStandardConditions(conds *[]metav1.Condition, state ConditionState, reason, message string) {
	stdconditions.SetStandardConditions(conds, state, reason, message)
}

// NeedStatusUpdate returns true if the Ready condition or reconcile fields differ from current status,
// so a status update should be persisted.
func NeedStatusUpdate(
	conditions []metav1.Condition,
	newReadyStatus metav1.ConditionStatus,
	newReason, newMessage string,
	newLastErr, newResourceStatus string,
	currentLastErr, currentResourceStatus string,
) bool {
	r := meta.FindStatusCondition(conditions, "Ready")
	if r == nil {
		return true
	}
	if r.Status != newReadyStatus || r.Reason != newReason || r.Message != newMessage {
		return true
	}
	if currentLastErr != newLastErr || currentResourceStatus != newResourceStatus {
		return true
	}
	return false
}

// ReconcileMeta returns ObservedGeneration and LastReconcileTime for the current reconcile.
// Assign to status at the start of reconcile: status.ObservedGeneration, status.LastReconcileTime = ReconcileMeta(obj.Generation).
func ReconcileMeta(generation int64) (observedGeneration int64, lastReconcileTime *metav1.Time) {
	now := metav1.Now()
	return generation, &now
}

// SuspendAnnotationKey is the annotation key to suspend reconciliation.
// When set to a truthy value (e.g. "true"), the controller skips reconciliation.
const SuspendAnnotationKey = "dbaas.controllers.thalassa.cloud/suspend"

// IsSuspended returns true if the object has the suspend annotation set to a truthy value.
func IsSuspended(obj metav1.Object) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}
	v := obj.GetAnnotations()[SuspendAnnotationKey]
	return strings.EqualFold(v, "true") || v == "1"
}

// effectiveName returns the name to use for the Thalassa resource: spec.metadata.name if set, otherwise defaultName.
func EffectiveName(defaultName string, resourceMeta *dbaasv1.ResourceMetadata) string {
	if resourceMeta != nil && resourceMeta.Name != nil && *resourceMeta.Name != "" {
		return *resourceMeta.Name
	}
	return defaultName
}

func EffectiveLabelsDbaas(resourceMeta *dbaasv1.ResourceMetadata) map[string]string {
	if resourceMeta == nil || len(resourceMeta.Labels) == 0 {
		return nil
	}
	return resourceMeta.Labels
}

func EffectiveAnnotationsDbaas(resourceMeta *dbaasv1.ResourceMetadata) map[string]string {
	if resourceMeta == nil || len(resourceMeta.Annotations) == 0 {
		return nil
	}
	return resourceMeta.Annotations
}

// QuantityFromDecimalGigabytes converts a decimal gigabyte value
// into a Kubernetes resource.Quantity. The suffix must be SI "G" (10^9 bytes), not "GB", which resource.ParseQuantity rejects.
func QuantityFromDecimalGigabytes(gb float64) resource.Quantity {
	switch {
	case math.IsNaN(gb), math.IsInf(gb, 0), gb <= 0:
		return resource.MustParse("0")
	default:
		s := strconv.FormatFloat(gb, 'f', -1, 64)
		q, err := resource.ParseQuantity(s + "G")
		if err != nil {
			return resource.MustParse("0")
		}
		return q
	}
}

// updateStatusWithRetry runs the status update in a retry loop using retry.OnError.
// The caller should pass a function that fetches the latest object, copies status from the in-memory object, and calls Status().Update.
func UpdateStatusWithRetry(doUpdate func() error) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, doUpdate)
}
