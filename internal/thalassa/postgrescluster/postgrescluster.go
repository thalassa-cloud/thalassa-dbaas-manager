package postgrescluster

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
	// Finalizer is the controller finalizer on PostgresCluster resources.
	Finalizer = "dbaas.controllers.thalassa.cloud/postgrescluster"
	// PreDeleteBackupIdentityAnnotation stores the Thalassa backup identity while BackupAndDelete waits for the final backup.
	PreDeleteBackupIdentityAnnotation = "dbaas.controllers.thalassa.cloud/pre-delete-backup-identity"

	servicePortRW = 5432
	servicePortRO = 5433
	// postgresClusterReadyMinDur is how long the cluster must stay ready before marking Available.
	postgresClusterReadyMinDur = 2 * time.Second

	// endpointSliceServiceNameLabel links an EndpointSlice to its Service (required by Kubernetes).
	endpointSliceServiceNameLabel = "kubernetes.io/service-name"

	requeueAfterStatusUpdateFailure = 15 * time.Second
)

// Config holds dependencies for Handler.
type Config struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	DbaasClient *dbaas.Client
	IaaSClient  *thalassaiaas.Client
	Recorder    record.EventRecorder
}

// ReconcileInput carries resolved inputs for a PostgresCluster reconcile (after the controller resolves subnet, SGs, etc.).
type ReconcileInput struct {
	PG             *dbaasv1.PostgresCluster
	SubnetIdentity string
	SGIdentities   []string
	ObjectStoreID  string
	EngineVersion  string
}

// Reconciler implements Thalassa PostgresCluster lifecycle: create/sync and termination.
type Reconciler interface {
	Reconcile(ctx context.Context, in ReconcileInput) (ctrl.Result, error)
	Terminate(ctx context.Context, pg *dbaasv1.PostgresCluster) (ctrl.Result, error)
}

// Handler implements Reconciler with Thalassa and Kubernetes clients.
type Handler struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	DbaasClient *dbaas.Client
	IaaSClient  *thalassaiaas.Client
	Recorder    record.EventRecorder
}

var _ Reconciler = (*Handler)(nil)

// NewHandler builds a Handler from Config.
func NewHandler(cfg Config) *Handler {
	return &Handler{
		Client:      cfg.Client,
		Scheme:      cfg.Scheme,
		DbaasClient: cfg.DbaasClient,
		IaaSClient:  cfg.IaaSClient,
		Recorder:    cfg.Recorder,
	}
}

// Reconcile creates or syncs the PostgresCluster in Thalassa.
func (h *Handler) Reconcile(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	pg := in.PG
	if len(pg.Spec.BackupSchedules) > 0 {
		if err := validateBackupScheduleTemplates(pg.Spec.BackupSchedules); err != nil {
			return h.setPostgresClusterErrorCondition(ctx, pg, "InvalidSpec", err.Error(), err)
		}
	}
	if pg.Status.ResourceID == "" {
		return h.createPostgresCluster(ctx, in)
	}
	return h.reconcilePostgresCluster(ctx, in)
}

// Terminate handles deletion: Thalassa cluster removal per onDelete policy and finalizer removal.
func (h *Handler) Terminate(ctx context.Context, pg *dbaasv1.PostgresCluster) (ctrl.Result, error) {
	return h.terminate(ctx, pg)
}
