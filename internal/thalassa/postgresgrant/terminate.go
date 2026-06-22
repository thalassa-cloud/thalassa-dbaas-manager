package postgresgrant

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func (h *Handler) terminate(ctx context.Context, grant *dbaasv1.PostgresGrant) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(grant, Finalizer) {
		return ctrl.Result{}, nil
	}
	clusterIdentity, err := h.resolvePostgresClusterRef(ctx, grant.Namespace, grant.Spec.ClusterRef)
	if err != nil && !errors.Is(err, ErrDependencyNotReady) {
		return ctrl.Result{}, err
	}
	if clusterIdentity != "" && grant.Status.ResourceID != "" {
		if err := h.DbaasClient.DeletePgGrant(ctx, clusterIdentity, grant.Status.ResourceID); err != nil && !thalassaclient.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted PostgreSQL grant in Thalassa", "identity", grant.Status.ResourceID)
		h.Recorder.Eventf(grant, corev1.EventTypeNormal, "Deleted", "Deleted PostgreSQL grant in Thalassa (%s)", grant.Status.ResourceID)
	}
	if controllerutil.RemoveFinalizer(grant, Finalizer) {
		if err := h.Client.Update(ctx, grant); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
