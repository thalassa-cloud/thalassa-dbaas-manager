// Package postgresclusterref resolves PostgresClusterRef to a Thalassa cluster identity.
package postgresclusterref

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	thalassahelpers "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/helpers"
)

// ErrDependencyNotReady is returned when the referenced PostgresCluster has no ResourceID yet.
var ErrDependencyNotReady = thalassahelpers.ErrDependencyNotReady

// Resolve returns the Thalassa cluster identity for ref (direct identity or PostgresCluster status).
func Resolve(ctx context.Context, c client.Client, defaultNamespace string, ref dbaasv1.PostgresClusterRef) (string, error) {
	if ref.Identity != "" {
		return ref.Identity, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("clusterRef.name or clusterRef.identity is required")
	}
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNamespace
	}
	var cluster dbaasv1.PostgresCluster
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &cluster); err != nil {
		return "", err
	}
	if cluster.Status.ResourceID == "" {
		return "", ErrDependencyNotReady
	}
	return cluster.Status.ResourceID, nil
}
