package postgresrole

import (
	"context"
	"fmt"
	"strings"

	"github.com/thalassa-cloud/client-go/dbaas"
	"github.com/thalassa-cloud/client-go/filters"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func isPgRoleAlreadyExists(err error) bool {
	if !thalassaclient.IsBadRequest(err) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

func (h *Handler) findPgRoleByName(ctx context.Context, clusterIdentity, name string) (*dbaas.DbClusterPostgresRole, error) {
	roles, err := h.DbaasClient.ListPgRoles(ctx, clusterIdentity, &dbaas.ListPgRolesRequest{
		Filters: []filters.Filter{
			&filters.FilterKeyValue{Key: filters.FilterName, Value: name},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list postgres roles: %w", err)
	}
	for i := range roles {
		if roles[i].Name == name {
			return &roles[i], nil
		}
	}
	return nil, nil
}

// createOrAdoptPgRole creates the role in Thalassa, or adopts an existing role with the same name
// when the API reports it already exists (e.g. status.resourceId was lost).
func (h *Handler) createOrAdoptPgRole(ctx context.Context, clusterIdentity string, role *dbaasv1.PostgresRole, password string) (*dbaas.DbClusterPostgresRole, bool, error) {
	created, err := h.DbaasClient.CreatePgRole(ctx, clusterIdentity, specToCreateRoleRequest(role, password))
	if err == nil {
		return created, false, nil
	}
	if !isPgRoleAlreadyExists(err) {
		return nil, false, err
	}
	existing, findErr := h.findPgRoleByName(ctx, clusterIdentity, role.Spec.Name)
	if findErr != nil {
		return nil, false, findErr
	}
	if existing == nil {
		return nil, false, fmt.Errorf("role %q already exists in Thalassa but could not be found", role.Spec.Name)
	}
	return existing, true, nil
}
