package dbobjectstore

import (
	"context"
	"time"

	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) updateStatusWithRetry(ctx context.Context, obj *dbaasv1.DbObjectStore) error {
	return retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		var latest dbaasv1.DbObjectStore
		if err := h.Client.Get(ctx, client.ObjectKeyFromObject(obj), &latest); err != nil {
			return err
		}
		latest.Status = obj.Status
		return h.Client.Status().Update(ctx, &latest)
	})
}

func (h *Handler) setErrorCondition(ctx context.Context, obj *dbaasv1.DbObjectStore, reason, message string, err error) (ctrl.Result, error) {
	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateDegraded, reason, message)
	obj.Status.LastReconcileError = message
	if updateErr := h.updateStatusWithRetry(ctx, obj); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	return ctrl.Result{RequeueAfter: time.Minute}, err
}
