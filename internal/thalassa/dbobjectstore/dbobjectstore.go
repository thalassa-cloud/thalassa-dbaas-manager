package dbobjectstore

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaiaas "github.com/thalassa-cloud/client-go/iaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

const (
	// Finalizer is the controller finalizer on DbObjectStore resources.
	Finalizer = "dbaas.controllers.thalassa.cloud/dbobjectstore"

	requeueAfterStatusUpdateFailure = 15 * time.Second
)

// Config holds dependencies for Handler.
type Config struct {
	Client          client.Client
	Scheme          *runtime.Scheme
	DbaasClient     *dbaas.Client
	IaaSClient      *thalassaiaas.Client
	DefaultRegion   string
	DefaultSubnetID string
	Recorder        record.EventRecorder
}

// ReconcileInput carries the object to reconcile (region is resolved inside the handler when needed).
type ReconcileInput struct {
	Obj *dbaasv1.DbObjectStore
}

// Reconciler implements Thalassa DbObjectStore lifecycle: create/sync and termination.
type Reconciler interface {
	Reconcile(ctx context.Context, in ReconcileInput) (ctrl.Result, error)
	Terminate(ctx context.Context, obj *dbaasv1.DbObjectStore) (ctrl.Result, error)
}

// Handler implements Reconciler with Thalassa and Kubernetes clients.
type Handler struct {
	Client          client.Client
	Scheme          *runtime.Scheme
	DbaasClient     *dbaas.Client
	IaaSClient      *thalassaiaas.Client
	DefaultRegion   string
	DefaultSubnetID string
	Recorder        record.EventRecorder
}

var _ Reconciler = (*Handler)(nil)

// NewHandler builds a Handler from Config.
func NewHandler(cfg Config) *Handler {
	return &Handler{
		Client:          cfg.Client,
		Scheme:          cfg.Scheme,
		DbaasClient:     cfg.DbaasClient,
		IaaSClient:      cfg.IaaSClient,
		DefaultRegion:   cfg.DefaultRegion,
		DefaultSubnetID: cfg.DefaultSubnetID,
		Recorder:        cfg.Recorder,
	}
}

// Reconcile creates or syncs the DB object store in Thalassa.
func (h *Handler) Reconcile(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	obj := in.Obj
	if obj.Status.ResourceID == "" {
		region, err := h.resolveObjectStoreRegion(ctx, obj)
		if err != nil {
			return h.setErrorCondition(ctx, obj, "RegionNotResolved", err.Error(), err)
		}
		return h.createObjectStore(ctx, obj, region)
	}
	return h.reconcileObjectStore(ctx, obj)
}

// Terminate deletes the object store in Thalassa and clears the finalizer.
func (h *Handler) Terminate(ctx context.Context, obj *dbaasv1.DbObjectStore) (ctrl.Result, error) {
	return h.terminate(ctx, obj)
}
