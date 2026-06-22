package postgresrole

import (
	"context"
	"time"

	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) updateStatusWithRetry(ctx context.Context, role *dbaasv1.PostgresRole) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		var latest dbaasv1.PostgresRole
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(role), &latest); err != nil {
			return err
		}
		latest.Status = role.Status
		return h.Client.Status().Update(ctx, &latest)
	})
}

func (h *Handler) setPostgresRoleErrorCondition(ctx context.Context, role *dbaasv1.PostgresRole, reason, message string) (ctrl.Result, error) {
	stdconditions.SetStandardConditions(&role.Status.Conditions, stdconditions.ConditionStateDegraded, reason, message)
	role.Status.LastReconcileError = message
	if updateErr := h.updateStatusWithRetry(ctx, role); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetErrorCondition sets a degraded Ready condition and LastReconcileError (e.g. cluster ref or password resolution failure before Reconcile).
func (h *Handler) SetErrorCondition(ctx context.Context, role *dbaasv1.PostgresRole, reason, message string, _ error) (ctrl.Result, error) {
	return h.setPostgresRoleErrorCondition(ctx, role, reason, message)
}
