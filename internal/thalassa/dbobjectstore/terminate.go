package dbobjectstore

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func (h *Handler) terminate(ctx context.Context, obj *dbaasv1.DbObjectStore) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(obj, Finalizer) {
		return ctrl.Result{}, nil
	}
	if obj.Status.ResourceID != "" {
		if err := h.DbaasClient.DeleteDbObjectStore(ctx, obj.Status.ResourceID); err != nil && !thalassaclient.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		log.Info("deleted DB object store in Thalassa", "identity", obj.Status.ResourceID)
		h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Deleted", "Deleted DB object store in Thalassa (%s)", obj.Status.ResourceID)
	}
	if controllerutil.RemoveFinalizer(obj, Finalizer) {
		if err := h.Client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
