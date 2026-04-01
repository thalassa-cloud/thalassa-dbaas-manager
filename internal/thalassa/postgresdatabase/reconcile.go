package postgresdatabase

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) createPostgresDatabase(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	db := in.DB
	clusterIdentity := in.ClusterIdentity

	stdconditions.SetStandardConditions(&db.Status.Conditions, stdconditions.ConditionStateProgressing, "Creating", "Creating PostgreSQL database in Thalassa")
	if updateErr := h.updateStatusWithRetry(ctx, db); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	req := specToCreateDatabaseRequest(db)
	created, err := h.DbaasClient.CreatePgDatabase(ctx, clusterIdentity, req)
	if err != nil {
		return h.setPostgresDatabaseErrorCondition(ctx, db, "FailedCreate", err.Error(), err)
	}
	h.Recorder.Eventf(db, corev1.EventTypeNormal, "Created", "Created PostgreSQL database in Thalassa (%s)", created.Identity)
	db.Status.ResourceID = created.Identity
	db.Status.ResourceStatus = string(created.Status)
	db.Status.LastReconcileError = ""
	h.setPostgresDatabaseConditionFromStatus(db, stdconditions.ResourceStatusReady, "Created")
	if updateErr := h.updateStatusWithRetry(ctx, db); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	log.Info("created PostgreSQL database in Thalassa", "identity", created.Identity)
	return ctrl.Result{}, nil
}

func (h *Handler) reconcilePostgresDatabase(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	db := in.DB
	clusterIdentity := in.ClusterIdentity
	identity := db.Status.ResourceID
	wasReady := meta.IsStatusConditionTrue(db.Status.Conditions, "Ready")
	req := specToUpdateDatabaseRequest(db)
	updated, err := h.DbaasClient.UpdatePgDatabase(ctx, clusterIdentity, identity, req)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			stdconditions.SetStandardConditions(&db.Status.Conditions, stdconditions.ConditionStateDegraded, "NotFound", "Database not found in Thalassa")
			db.Status.ResourceID = ""
			db.Status.ResourceStatus = ""
			if updateErr := h.updateStatusWithRetry(ctx, db); updateErr != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		return h.setPostgresDatabaseErrorCondition(ctx, db, "FailedUpdate", err.Error(), err)
	}
	h.Recorder.Eventf(db, corev1.EventTypeNormal, "Updated", "Updated PostgreSQL database in Thalassa (%s)", identity)
	db.Status.ResourceStatus = string(updated.Status)
	db.Status.LastReconcileError = ""
	h.setPostgresDatabaseConditionFromStatus(db, stdconditions.ResourceStatusReady, "Synced")
	isReady := meta.IsStatusConditionTrue(db.Status.Conditions, "Ready")
	if !wasReady && isReady {
		h.Recorder.Event(db, corev1.EventTypeNormal, "Ready", "PostgreSQL database is Ready")
	}
	if updateErr := h.updateStatusWithRetry(ctx, db); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func specToCreateDatabaseRequest(db *dbaasv1.PostgresDatabase) dbaas.CreatePgDatabaseRequest {
	req := dbaas.CreatePgDatabaseRequest{
		Name:  db.Spec.Name,
		Owner: db.Spec.Owner,
	}
	if db.Spec.AllowConnections != nil {
		req.AllowConnections = db.Spec.AllowConnections
	}
	if db.Spec.ConnectionLimit != nil {
		req.ConnectionLimit = db.Spec.ConnectionLimit
	}
	if len(db.Spec.Extensions) > 0 {
		exts := make([]dbaas.DatabasePostgresExtension, 0, len(db.Spec.Extensions))
		for _, e := range db.Spec.Extensions {
			exts = append(exts, dbaas.DatabasePostgresExtension{Name: e.Name})
		}
		req.Extensions = &dbaas.PgDatabaseExtensions{Extensions: exts}
	}
	return req
}

func specToUpdateDatabaseRequest(db *dbaasv1.PostgresDatabase) dbaas.UpdatePgDatabaseRequest {
	req := dbaas.UpdatePgDatabaseRequest{}
	if db.Spec.AllowConnections != nil {
		req.AllowConnections = db.Spec.AllowConnections
	}
	if db.Spec.ConnectionLimit != nil {
		req.ConnectionLimit = db.Spec.ConnectionLimit
	}
	if len(db.Spec.Extensions) > 0 {
		exts := make([]dbaas.DatabasePostgresExtension, 0, len(db.Spec.Extensions))
		for _, e := range db.Spec.Extensions {
			exts = append(exts, dbaas.DatabasePostgresExtension{Name: e.Name})
		}
		req.Extensions = &dbaas.PgDatabaseExtensions{Extensions: exts}
	}
	return req
}

// setPostgresDatabaseConditionFromStatus sets Available/Ready from the database resource status (DBaaS ObjectStatus).
// When status is unknown (e.g. CreatePgDatabase does not return status), call with ResourceStatusReady after success.
func (h *Handler) setPostgresDatabaseConditionFromStatus(db *dbaasv1.PostgresDatabase, resourceStatus string, reason string) {
	switch {
	case strings.EqualFold(resourceStatus, stdconditions.ResourceStatusReady):
		stdconditions.SetStandardConditions(&db.Status.Conditions, stdconditions.ConditionStateAvailable, reason, "PostgreSQL database is ready")
	case strings.EqualFold(resourceStatus, string(dbaas.ObjectStatusFailed)),
		strings.EqualFold(resourceStatus, string(dbaas.ObjectStatusDeleted)):
		stdconditions.SetStandardConditions(&db.Status.Conditions, stdconditions.ConditionStateDegraded, "DatabaseNotReady", "Database status: "+resourceStatus)
	default:
		stdconditions.SetStandardConditions(&db.Status.Conditions, stdconditions.ConditionStateProgressing, "DatabaseNotReady", "Database status: "+resourceStatus)
	}
}
