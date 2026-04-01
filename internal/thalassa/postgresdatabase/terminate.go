package postgresdatabase

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

func (h *Handler) terminate(ctx context.Context, db *dbaasv1.PostgresDatabase) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(db, Finalizer) {
		return ctrl.Result{}, nil
	}
	clusterIdentity, err := h.resolvePostgresClusterRef(ctx, db.Namespace, db.Spec.ClusterRef)
	if err != nil && !errors.Is(err, ErrDependencyNotReady) {
		return ctrl.Result{}, err
	}
	if clusterIdentity != "" && db.Status.ResourceID != "" {
		if err := h.DbaasClient.DeletePgDatabase(ctx, clusterIdentity, db.Status.ResourceID, false); err != nil && !thalassaclient.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted PostgreSQL database in Thalassa", "identity", db.Status.ResourceID)
		h.Recorder.Eventf(db, corev1.EventTypeNormal, "Deleted", "Deleted PostgreSQL database in Thalassa (%s)", db.Status.ResourceID)
	}
	if controllerutil.RemoveFinalizer(db, Finalizer) {
		if err := h.Client.Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
