package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

// enqueueRestoresForSourceCluster requeues clusters that restore from source once it has a Thalassa identity.
func (r *PostgresClusterReconciler) enqueueRestoresForSourceCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	source, ok := obj.(*dbaasv1.PostgresCluster)
	if !ok || source.Status.ResourceID == "" {
		return nil
	}

	var list dbaasv1.PostgresClusterList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		item := &list.Items[i]
		if item.Namespace == source.Namespace && item.Name == source.Name {
			continue
		}
		if !restoreReferencesSource(item, source) {
			continue
		}
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: item.Namespace, Name: item.Name},
		})
	}
	return reqs
}

func restoreReferencesSource(item, source *dbaasv1.PostgresCluster) bool {
	if item.Spec.Restore == nil || item.Spec.Restore.SourceClusterRef == nil || source == nil {
		return false
	}
	ref := item.Spec.Restore.SourceClusterRef
	if ref.Identity != "" {
		return ref.Identity == source.Status.ResourceID
	}
	if ref.Name == "" {
		return false
	}
	ns := ref.Namespace
	if ns == "" {
		ns = item.Namespace
	}
	return ref.Name == source.Name && ns == source.Namespace
}

func restoreSourcePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*dbaasv1.PostgresCluster)
			return ok && cluster.Status.ResourceID != ""
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, okOld := e.ObjectOld.(*dbaasv1.PostgresCluster)
			newCluster, okNew := e.ObjectNew.(*dbaasv1.PostgresCluster)
			if !okOld || !okNew {
				return false
			}
			return oldCluster.Status.ResourceID == "" && newCluster.Status.ResourceID != ""
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
