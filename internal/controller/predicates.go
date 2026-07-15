package controller

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// PrimaryResourcePredicate selects events for the primary CR that should trigger reconciliation.
//
// It intentionally ignores most metadata/status-only updates (generation unchanged) to avoid
// reconcile storms, but still fires when:
//   - the Spec changes (generation bump),
//   - a deletionTimestamp is set (finalizer / terminate path),
//   - the suspend annotation is added or removed (resume without a Spec change).
//
// DeleteFunc is false: once the object is gone from the cache, Get returns NotFound.
func PrimaryResourcePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc:  primaryResourceUpdate,
	}
}

// OwnedResourcePredicate selects events for secondary resources (.Owns) so the owner
// controller can self-heal. Delete must be true so removing a Service/EndpointSlice/Secret
// re-enqueues the owner immediately rather than waiting for a long RequeueAfter.
func OwnedResourcePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			// Spec drift on owned objects (e.g. Service ports). Status-only noise is ignored.
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
	}
}

func primaryResourceUpdate(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}
	if !e.ObjectNew.GetDeletionTimestamp().IsZero() {
		return true
	}
	if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
		return true
	}
	return IsSuspended(e.ObjectOld) != IsSuspended(e.ObjectNew)
}
