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

package controller

import (
	"context"
	"fmt"

	"github.com/thalassa-cloud/client-go/dbaas"
	"github.com/thalassa-cloud/client-go/filters"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

// resolveDbObjectStoreIdentity resolves spec.dbObjectStore (DbObjectStoreRef) to a Thalassa object store identity.
// Precedence: identity → label selector (DBaaS API) → DbObjectStore CR (TypedObjectReference) → manager default-dbobjectstore-id.
// An empty return value means do not set DbObjectStoreIdentity on the API request (no default configured).
func (r *PostgresClusterReconciler) resolveDbObjectStoreIdentity(ctx context.Context, defaultNamespace string, pg *dbaasv1.PostgresCluster) (string, error) {
	ref := pg.Spec.DbObjectStoreRef
	if ref == nil {
		return r.defaultDbObjectStoreIDOrEmpty(), nil
	}
	if ref.Identity != "" {
		return ref.Identity, nil
	}
	if ref.Selector != nil {
		return r.resolveDbObjectStoreBySelector(ctx, ref.Selector)
	}
	if ref.DbObjectStoreRef != nil {
		id, err := r.resourceIDFromTypedRef(ctx, defaultNamespace, ref.DbObjectStoreRef)
		if err != nil {
			return "", fmt.Errorf("db object store typed reference: %w", err)
		}
		return id, nil
	}
	return r.defaultDbObjectStoreIDOrEmpty(), nil
}

func (r *PostgresClusterReconciler) defaultDbObjectStoreIDOrEmpty() string {
	return r.DefaultDbObjectStoreID
}

func (r *PostgresClusterReconciler) resolveDbObjectStoreBySelector(ctx context.Context, sel *metav1.LabelSelector) (string, error) {
	if r.DbaasClient == nil {
		return "", fmt.Errorf("DBaaS client not configured for DbObjectStore label selector")
	}
	selAsLabels, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return "", fmt.Errorf("db object store label selector: %w", err)
	}

	var list []dbaas.DbObjectStore
	if len(sel.MatchExpressions) == 0 && len(sel.MatchLabels) > 0 {
		list, err = r.DbaasClient.ListDbObjectStores(ctx, &dbaas.ListDbObjectStoresRequest{
			Filters: []filters.Filter{&filters.LabelFilter{MatchLabels: sel.MatchLabels}},
		})
	} else {
		list, err = r.DbaasClient.ListDbObjectStores(ctx, nil)
	}
	if err != nil {
		return "", fmt.Errorf("list db object stores: %w", err)
	}

	var matches []string
	for i := range list {
		if selAsLabels.Matches(labels.Set(list[i].Labels)) {
			matches = append(matches, list[i].Identity)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no DbObjectStore matches label selector")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple DbObjectStores (%d) match label selector", len(matches))
	}
}

// dbObjectStoreSpecDrift reports whether the cluster's object store should be updated in Thalassa.
// When the spec resolves to no identity but the cluster already has one, we return false: UpdateDbClusterRequest
// omits nil DbObjectStoreIdentity, so the API would not clear it and reconcile would loop.
func dbObjectStoreSpecDrift(desiredIdentity, fetchedIdentity string) bool {
	if desiredIdentity == fetchedIdentity {
		return false
	}
	if desiredIdentity == "" && fetchedIdentity != "" {
		return false
	}
	return true
}
