package postgresgrant

import (
	"context"
	"time"

	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) updateStatusWithRetry(ctx context.Context, grant *dbaasv1.PostgresGrant) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		var latest dbaasv1.PostgresGrant
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(grant), &latest); err != nil {
			return err
		}
		latest.Status = grant.Status
		return h.Client.Status().Update(ctx, &latest)
	})
}

func (h *Handler) setPostgresGrantErrorCondition(ctx context.Context, grant *dbaasv1.PostgresGrant, reason, message string, err error) (ctrl.Result, error) {
	stdconditions.SetStandardConditions(&grant.Status.Conditions, stdconditions.ConditionStateDegraded, reason, message)
	grant.Status.LastReconcileError = message
	if updateErr := h.updateStatusWithRetry(ctx, grant); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
}

// SetErrorCondition sets a degraded Ready condition and LastReconcileError (e.g. when the controller cannot resolve cluster ref before Reconcile).
func (h *Handler) SetErrorCondition(ctx context.Context, grant *dbaasv1.PostgresGrant, reason, message string, reconcileErr error) (ctrl.Result, error) {
	return h.setPostgresGrantErrorCondition(ctx, grant, reason, message, reconcileErr)
}
