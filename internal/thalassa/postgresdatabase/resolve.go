package postgresdatabase

import (
	"context"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	pgref "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresclusterref"
)

// ErrDependencyNotReady matches postgresclusterref.ErrDependencyNotReady for callers comparing with errors.Is.
var ErrDependencyNotReady = pgref.ErrDependencyNotReady

func (h *Handler) resolvePostgresClusterRef(ctx context.Context, defaultNamespace string, ref dbaasv1.PostgresClusterRef) (string, error) {
	return pgref.Resolve(ctx, h.Client, defaultNamespace, ref)
}
