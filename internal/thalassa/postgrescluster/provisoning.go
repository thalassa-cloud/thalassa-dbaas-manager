package postgrescluster

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) createPostgresCluster(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	pg := in.PG

	stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, "Creating", "Creating PostgreSQL cluster in Thalassa")
	if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}

	createReq := h.specToCreateRequest(pg, in.SubnetIdentity, in.SGIdentities, in.EngineVersion, in.ObjectStoreID)

	volumeTypeIdentity, err := h.resolveVolumeTypeClassID(ctx, pg.Spec.VolumeTypeClassId)
	if err != nil {
		return h.setPostgresClusterErrorCondition(ctx, pg, "FailedResolveVolumeTypeClassId", err.Error(), err)
	}
	createReq.VolumeTypeClassIdentity = volumeTypeIdentity

	created, err := h.DbaasClient.CreateDbCluster(ctx, createReq)
	if err != nil {
		return h.setPostgresClusterErrorCondition(ctx, pg, "FailedCreate", err.Error(), err)
	}
	h.Recorder.Eventf(pg, corev1.EventTypeNormal, "Created", "Created PostgreSQL cluster in Thalassa (%s)", created.Identity)

	pg.Status.ResourceID = created.Identity
	pg.Status.ResourceStatus = string(created.Status)
	pg.Status.EngineVersion = created.EngineVersion
	pg.Status.LastReconcileError = ""
	if created.EndpointIpv4 != "" {
		pg.Status.EndpointHost = created.EndpointIpv4
		pg.Status.Port = int32(created.Port)
		if pg.Status.Port == 0 {
			pg.Status.Port = servicePortRW
		}
		pg.Status.ReadOnlyPort = servicePortRO
	}
	if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	log.Info("created PostgreSQL cluster in Thalassa", "identity", created.Identity)

	h.setPostgresClusterConditionFromStatus(pg, string(created.Status), "Created")
	if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}

	if err := h.reconcileEmbeddedBackupSchedules(ctx, created.Identity, pg); err != nil {
		return h.setPostgresClusterErrorCondition(ctx, pg, "BackupScheduleReconcileFailed", err.Error(), err)
	}
	if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}

	if err := h.reconcileExposeService(ctx, pg); err != nil {
		log.Error(err, "failed to reconcile expose Service, will retry")
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, nil
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
