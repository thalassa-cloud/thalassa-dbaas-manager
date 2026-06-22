package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func (r *PostgresRoleReconciler) enqueuePostgresRolesForCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	cluster, ok := obj.(*dbaasv1.PostgresCluster)
	if !ok {
		return nil
	}

	var roleList dbaasv1.PostgresRoleList
	if err := r.List(ctx, &roleList, client.InNamespace(cluster.Namespace)); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range roleList.Items {
		role := &roleList.Items[i]
		if role.Spec.WriteConnectionSecretToRef == nil {
			continue
		}
		clusterNS := role.Spec.ClusterRef.Namespace
		if clusterNS == "" {
			clusterNS = role.Namespace
		}
		if clusterNS == cluster.Namespace && role.Spec.ClusterRef.Name == cluster.Name {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: role.Namespace, Name: role.Name},
			})
		}
	}
	return reqs
}
