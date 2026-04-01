package postgrescluster

import (
	"context"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thalassa-cloud/client-go/dbaas"
	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) setPostgresClusterConditionWithStability(pg *dbaasv1.PostgresCluster) {
	status := pg.Status.ResourceStatus
	now := metav1.Now()

	if !strings.EqualFold(status, stdconditions.ResourceStatusReady) {
		pg.Status.ReadyObservedAt = nil
		h.setPostgresClusterConditionFromStatus(pg, status, "Synced")
		return
	}
	if pg.Status.ReadyObservedAt == nil {
		pg.Status.ReadyObservedAt = &now
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, "ClusterNotReady", "Cluster is ready; waiting for stable duration before marking Available")
		return
	}
	elapsed := time.Since(pg.Status.ReadyObservedAt.Time)
	if elapsed >= postgresClusterReadyMinDur {
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateAvailable, "Synced", "PostgreSQL cluster is ready")
	} else {
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, "ClusterNotReady", "Cluster is ready; waiting for stable duration before marking Available")
	}
}

func (h *Handler) setPostgresClusterConditionFromStatus(pg *dbaasv1.PostgresCluster, resourceStatus string, reason string) {
	switch {
	case strings.EqualFold(resourceStatus, stdconditions.ResourceStatusReady):
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateAvailable, reason, "PostgreSQL cluster is ready")
	case strings.EqualFold(resourceStatus, string(dbaas.DbClusterStatusFailed)),
		strings.EqualFold(resourceStatus, string(dbaas.DbClusterStatusDeleted)),
		strings.EqualFold(resourceStatus, string(dbaas.DbClusterStatusUnknown)):
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateDegraded, "ClusterNotReady", "DB cluster status: "+resourceStatus)
	default:
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, "ClusterNotReady", "DB cluster status: "+resourceStatus)
	}
}

func (h *Handler) updateStatusWithRetry(ctx context.Context, pg *dbaasv1.PostgresCluster) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		var latest dbaasv1.PostgresCluster
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(pg), &latest); err != nil {
			return err
		}
		latest.Status = pg.Status
		return h.Client.Status().Update(ctx, &latest)
	})
}

func (h *Handler) setPostgresClusterErrorCondition(ctx context.Context, pg *dbaasv1.PostgresCluster, reason, message string, err error) (ctrl.Result, error) {
	stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateDegraded, reason, message)
	pg.Status.LastReconcileError = message
	if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
}
