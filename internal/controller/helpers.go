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

// Package controller re-exports shared helpers from internal/thalassa/helpers for reconcilers in this package.
package controller

import (
	thalassahelpers "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/helpers"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RequeueAfterStatusUpdateFailure = thalassahelpers.RequeueAfterStatusUpdateFailure
	RequeueAfterDependencyNotReady  = thalassahelpers.RequeueAfterDependencyNotReady
	ResourceStatusReady             = thalassahelpers.ResourceStatusReady
)

var ErrDependencyNotReady = thalassahelpers.ErrDependencyNotReady

type ConditionState = thalassahelpers.ConditionState

const (
	ConditionStateAvailable   = thalassahelpers.ConditionStateAvailable
	ConditionStateDegraded    = thalassahelpers.ConditionStateDegraded
	ConditionStateProgressing = thalassahelpers.ConditionStateProgressing
)

func SetStandardConditions(conds *[]metav1.Condition, state ConditionState, reason, message string) {
	thalassahelpers.SetStandardConditions(conds, state, reason, message)
}

func NeedStatusUpdate(
	conditions []metav1.Condition,
	newReadyStatus metav1.ConditionStatus,
	newReason, newMessage string,
	newLastErr, newResourceStatus string,
	currentLastErr, currentResourceStatus string,
) bool {
	return thalassahelpers.NeedStatusUpdate(conditions, newReadyStatus, newReason, newMessage, newLastErr, newResourceStatus, currentLastErr, currentResourceStatus)
}

func ReconcileMeta(generation int64) (observedGeneration int64, lastReconcileTime *metav1.Time) {
	return thalassahelpers.ReconcileMeta(generation)
}

func IsSuspended(obj metav1.Object) bool {
	return thalassahelpers.IsSuspended(obj)
}

func QuantityFromDecimalGigabytes(gb float64) resource.Quantity {
	return thalassahelpers.QuantityFromDecimalGigabytes(gb)
}

func updateStatusWithRetry(doUpdate func() error) error {
	return thalassahelpers.UpdateStatusWithRetry(doUpdate)
}
