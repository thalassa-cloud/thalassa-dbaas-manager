package postgresrole

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	pgref "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresclusterref"
)

func (h *Handler) terminate(ctx context.Context, role *dbaasv1.PostgresRole) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(role, Finalizer) {
		return ctrl.Result{}, nil
	}
	clusterIdentity, err := pgref.Resolve(ctx, h.Client, role.Namespace, role.Spec.ClusterRef)
	if err != nil && !errors.Is(err, pgref.ErrDependencyNotReady) && !kubeerrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if clusterIdentity != "" && role.Status.ResourceID != "" {
		if err := h.DbaasClient.DeletePgRole(ctx, clusterIdentity, role.Status.ResourceID); err != nil && !thalassaclient.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted PostgreSQL role in Thalassa", "identity", role.Status.ResourceID)
		h.Recorder.Eventf(role, corev1.EventTypeNormal, "Deleted", "Deleted PostgreSQL role in Thalassa (%s)", role.Status.ResourceID)
	}
	if controllerutil.RemoveFinalizer(role, Finalizer) {
		if err := h.Client.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
