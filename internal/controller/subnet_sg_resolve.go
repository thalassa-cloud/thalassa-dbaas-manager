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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/thalassa-cloud/client-go/filters"
	thalassaiaas "github.com/thalassa-cloud/client-go/iaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

// resolveSubnetIdentity resolves spec.subnet (SubnetRef) to a Thalassa subnet identity.
// Precedence: identity → label selector (Thalassa API) → IaaS Subnet CR (TypedObjectReference) → manager default-subnet-id.
func (r *PostgresClusterReconciler) resolveSubnetIdentity(ctx context.Context, defaultNamespace string, pg *dbaasv1.PostgresCluster) (string, error) {
	ref := pg.Spec.SubnetRef
	if ref == nil {
		return r.defaultSubnetOrErr()
	}
	if ref.Identity != "" {
		return ref.Identity, nil
	}
	if ref.Selector != nil {
		id, err := r.resolveSubnetBySelector(ctx, ref.Selector)
		if err != nil {
			return "", err
		}
		return id, nil
	}
	if ref.SubnetRef != nil {
		id, err := r.resourceIDFromTypedRef(ctx, defaultNamespace, ref.SubnetRef)
		if err != nil {
			return "", fmt.Errorf("subnet typed reference: %w", err)
		}
		return id, nil
	}
	return r.defaultSubnetOrErr()
}

func (r *PostgresClusterReconciler) defaultSubnetOrErr() (string, error) {
	if r.DefaultSubnetID == "" {
		return "", fmt.Errorf("no subnet specified and manager default-subnet-id is not set")
	}
	return r.DefaultSubnetID, nil
}

func (r *PostgresClusterReconciler) resolveSubnetBySelector(ctx context.Context, sel *metav1.LabelSelector) (string, error) {
	if r.IaaSClient == nil {
		return "", fmt.Errorf("IaaS client not configured for subnet label selector")
	}
	selAsLabels, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return "", fmt.Errorf("subnet label selector: %w", err)
	}

	var list []thalassaiaas.Subnet
	if len(sel.MatchExpressions) == 0 && len(sel.MatchLabels) > 0 {
		list, err = r.IaaSClient.ListSubnets(ctx, &thalassaiaas.ListSubnetsRequest{
			Filters: []filters.Filter{&filters.LabelFilter{MatchLabels: sel.MatchLabels}},
		})
	} else {
		list, err = r.IaaSClient.ListSubnets(ctx, nil)
	}
	if err != nil {
		return "", fmt.Errorf("list subnets: %w", err)
	}

	var matches []string
	for i := range list {
		if selAsLabels.Matches(labels.Set(list[i].Labels)) {
			matches = append(matches, list[i].Identity)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no subnet matches label selector")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple subnets (%d) match label selector", len(matches))
	}
}

// resolveSecurityGroupRefs resolves spec.securityGroups to Thalassa security group identities.
// When the slice is empty and DefaultSecurityGroupID is set, returns a single-element slice with that identity.
func (r *PostgresClusterReconciler) resolveSecurityGroupRefs(ctx context.Context, defaultNamespace string, refs []dbaasv1.SecurityGroupRef) ([]string, error) {
	if len(refs) == 0 {
		if r.DefaultSecurityGroupID != "" {
			return []string{r.DefaultSecurityGroupID}, nil
		}
		return nil, nil
	}
	out := make([]string, 0, len(refs))
	for i := range refs {
		id, err := r.resolveOneSecurityGroup(ctx, defaultNamespace, &refs[i])
		if err != nil {
			if err == ErrDependencyNotReady {
				return nil, err
			}
			return nil, fmt.Errorf("security group ref[%d]: %w", i, err)
		}
		out = append(out, id)
	}
	return out, nil
}

// resolveOneSecurityGroup precedence: identity → selector → TypedObjectReference.
func (r *PostgresClusterReconciler) resolveOneSecurityGroup(ctx context.Context, defaultNamespace string, ref *dbaasv1.SecurityGroupRef) (string, error) {
	if ref.Identity != "" {
		return ref.Identity, nil
	}
	if ref.Selector != nil {
		return r.resolveSecurityGroupBySelector(ctx, ref.Selector)
	}
	if ref.SecurityGroupRef != nil {
		return r.resourceIDFromTypedRef(ctx, defaultNamespace, ref.SecurityGroupRef)
	}
	return "", fmt.Errorf("security group ref must set identity, selector, or securityGroupRef")
}

func (r *PostgresClusterReconciler) resolveSecurityGroupBySelector(ctx context.Context, sel *metav1.LabelSelector) (string, error) {
	if r.IaaSClient == nil {
		return "", fmt.Errorf("IaaS client not configured for security group label selector")
	}
	selAsLabels, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return "", fmt.Errorf("security group label selector: %w", err)
	}

	var list []thalassaiaas.SecurityGroup
	if len(sel.MatchExpressions) == 0 && len(sel.MatchLabels) > 0 {
		list, err = r.IaaSClient.ListSecurityGroups(ctx, &thalassaiaas.ListSecurityGroupsRequest{
			Filters: []filters.Filter{&filters.LabelFilter{MatchLabels: sel.MatchLabels}},
		})
	} else {
		list, err = r.IaaSClient.ListSecurityGroups(ctx, nil)
	}
	if err != nil {
		return "", fmt.Errorf("list security groups: %w", err)
	}

	var matches []string
	for i := range list {
		if selAsLabels.Matches(labels.Set(list[i].Labels)) {
			matches = append(matches, list[i].Identity)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no security group matches label selector")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple security groups (%d) match label selector", len(matches))
	}
}

func gvkFromTypedRef(ref *corev1.TypedObjectReference) (schema.GroupVersionKind, error) {
	if ref == nil {
		return schema.GroupVersionKind{}, fmt.Errorf("typed reference is nil")
	}
	if ref.APIGroup == nil || *ref.APIGroup == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("typed reference requires apiGroup")
	}
	if ref.Kind == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("typed reference requires kind")
	}
	return schema.GroupVersionKind{Group: *ref.APIGroup, Version: "v1", Kind: ref.Kind}, nil
}

// resourceIDFromTypedRef loads an object (e.g. iaas.controllers.thalassa.cloud Subnet or SecurityGroup) and reads status.resourceId.
func (r *PostgresClusterReconciler) resourceIDFromTypedRef(ctx context.Context, defaultNamespace string, ref *corev1.TypedObjectReference) (string, error) {
	gvk, err := gvkFromTypedRef(ref)
	if err != nil {
		return "", err
	}
	if ref.Name == "" {
		return "", fmt.Errorf("typed reference requires name")
	}
	ns := defaultNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		ns = *ref.Namespace
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	key := types.NamespacedName{Namespace: ns, Name: ref.Name}
	if err := r.Get(ctx, key, u); err != nil {
		return "", err
	}
	rid, found, err := unstructured.NestedString(u.Object, "status", "resourceId")
	if err != nil {
		return "", err
	}
	if found && rid != "" {
		return rid, nil
	}
	rid2, found2, err := unstructured.NestedString(u.Object, "status", "resourceID")
	if err != nil {
		return "", err
	}
	if found2 && rid2 != "" {
		return rid2, nil
	}
	return "", ErrDependencyNotReady
}
