package postgrescluster

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) reconcilePostgresCluster(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	pg := in.PG
	identity := pg.Status.ResourceID

	fetched, err := h.DbaasClient.GetDbCluster(ctx, identity)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateDegraded, "NotFound", "Cluster not found in Thalassa")
			if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
			}
			return h.setPostgresClusterErrorCondition(ctx, pg, "NotFound", "Cluster not found in Thalassa", err)
		}
		return h.setPostgresClusterErrorCondition(ctx, pg, "GetFailed", "Failed to get cluster from Thalassa", err)
	}

	pg.Status.ResourceStatus = string(fetched.Status)
	pg.Status.EngineVersion = fetched.EngineVersion
	if fetched.EndpointIpv4 != "" {
		pg.Status.EndpointHost = fetched.EndpointIpv4
		pg.Status.Port = int32(fetched.Port)
		if pg.Status.Port == 0 {
			pg.Status.Port = servicePortRW
		}
		pg.Status.ReadOnlyPort = servicePortRO
	}

	pg.Status.Instances = make([]dbaasv1.PostgresClusterInstanceStatus, 0, len(fetched.DatabaseInstancesStatus.Instances))
	for _, instance := range fetched.DatabaseInstancesStatus.Instances {
		pg.Status.Instances = append(pg.Status.Instances, dbaasv1.PostgresClusterInstanceStatus{
			Name:             instance.Name,
			IsPrimary:        instance.IsPrimary,
			IsPrimaryTarget:  instance.IsPrimaryTarget,
			Healthy:          instance.Healthy,
			AllocatedStorage: instance.AllocatedStorage,
			Version:          instance.Version,
			AvailabilityZone: instance.AvailabilityZone,
			TimeLineID:       instance.TimeLineID,
			Replicating:      instance.Replicating,
			Joining:          instance.Joining,
			UsedStorage:      instance.UsedStorage,
			Memory:           instance.Memory,
			Cpu:              instance.Cpu,
		})
	}
	pg.Status.InstanceCount = len(fetched.DatabaseInstancesStatus.Instances)
	pg.Status.LastReconcileError = ""

	if err := h.reconcileEmbeddedBackupSchedules(ctx, identity, pg); err != nil {
		return h.setPostgresClusterErrorCondition(ctx, pg, "BackupScheduleReconcileFailed", err.Error(), err)
	}

	if h.requiresUpdate(pg, fetched, in.SGIdentities, in.ObjectStoreID) {
		log.Info("updating PostgreSQL cluster in Thalassa", "identity", identity)
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, "Updating", "Updating PostgreSQL cluster in Thalassa")
		if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
			return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
		}
		updateReq := h.specToUpdateRequest(pg, in.SGIdentities, in.ObjectStoreID)
		_, err = h.DbaasClient.UpdateDbCluster(ctx, identity, updateReq)
		if err != nil {
			return h.setPostgresClusterErrorCondition(ctx, pg, "FailedUpdate", err.Error(), err)
		}
		h.Recorder.Eventf(pg, corev1.EventTypeNormal, "Updated", "Updated PostgreSQL cluster in Thalassa (%s)", identity)
		pg.Status.ReadyObservedAt = nil
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, "ClusterNotReady", "Cluster was just updated; re-checking readiness")
		if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
			return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := h.reconcileExposeService(ctx, pg); err != nil {
		log.Error(err, "failed to reconcile expose Service, will retry")
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, nil
	}

	wasReady := meta.IsStatusConditionTrue(pg.Status.Conditions, "Ready")
	h.setPostgresClusterConditionWithStability(pg)
	isReady := meta.IsStatusConditionTrue(pg.Status.Conditions, "Ready")
	if !wasReady && isReady {
		h.Recorder.Event(pg, corev1.EventTypeNormal, "Ready", "PostgreSQL cluster is Ready")
	}
	if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	requeueAfter := 5 * time.Minute
	if !strings.EqualFold(pg.Status.ResourceStatus, stdconditions.ResourceStatusReady) {
		requeueAfter = 30 * time.Second
	} else if pg.Status.ReadyObservedAt != nil {
		elapsed := time.Since(pg.Status.ReadyObservedAt.Time)
		if elapsed < postgresClusterReadyMinDur {
			requeueAfter = max(postgresClusterReadyMinDur-elapsed, time.Second)
		}
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
