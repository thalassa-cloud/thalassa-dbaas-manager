package postgresgrant

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/thalassa-cloud/client-go/dbaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

const (
	// Finalizer is the controller finalizer on PostgresGrant resources.
	Finalizer = "dbaas.controllers.thalassa.cloud/postgresgrant"

	requeueAfterStatusUpdateFailure = 15 * time.Second
)

// Config holds dependencies for Handler.
type Config struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	DbaasClient *dbaas.Client
	Recorder    record.EventRecorder
}

// ReconcileInput carries resolved inputs after the controller resolves cluster reference.
type ReconcileInput struct {
	Grant           *dbaasv1.PostgresGrant
	ClusterIdentity string
}

// Reconciler implements Thalassa PostgresGrant lifecycle: create/sync and termination.
type Reconciler interface {
	Reconcile(ctx context.Context, in ReconcileInput) (ctrl.Result, error)
	Terminate(ctx context.Context, grant *dbaasv1.PostgresGrant) (ctrl.Result, error)
}

// Handler implements Reconciler with Thalassa and Kubernetes clients.
type Handler struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	DbaasClient *dbaas.Client
	Recorder    record.EventRecorder
}

var _ Reconciler = (*Handler)(nil)

// NewHandler builds a Handler from Config.
func NewHandler(cfg Config) *Handler {
	return &Handler{
		Client:      cfg.Client,
		Scheme:      cfg.Scheme,
		DbaasClient: cfg.DbaasClient,
		Recorder:    cfg.Recorder,
	}
}

// Reconcile creates or syncs the PostgreSQL grant in Thalassa.
func (h *Handler) Reconcile(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	if in.Grant.Status.ResourceID == "" {
		return h.createPostgresGrant(ctx, in)
	}
	return h.reconcilePostgresGrant(ctx, in)
}

// Terminate deletes the grant in Thalassa and clears the finalizer.
func (h *Handler) Terminate(ctx context.Context, grant *dbaasv1.PostgresGrant) (ctrl.Result, error) {
	return h.terminate(ctx, grant)
}
