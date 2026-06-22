package postgresgrant

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func (h *Handler) createPostgresGrant(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	grant := in.Grant
	clusterIdentity := in.ClusterIdentity

	stdconditions.SetStandardConditions(&grant.Status.Conditions, stdconditions.ConditionStateProgressing, "Creating", "Creating PostgreSQL grant in Thalassa")
	if updateErr := h.updateStatusWithRetry(ctx, grant); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	req := specToCreateGrantRequest(grant)
	created, err := h.DbaasClient.CreatePgGrant(ctx, clusterIdentity, req)
	if err != nil {
		return h.setPostgresGrantErrorCondition(ctx, grant, "FailedCreate", err.Error(), err)
	}
	h.Recorder.Eventf(grant, corev1.EventTypeNormal, "Created", "Created PostgreSQL grant in Thalassa (%s)", created.Identity)
	grant.Status.ResourceID = created.Identity
	grant.Status.ResourceStatus = string(created.Status)
	grant.Status.LastReconcileError = ""
	setPostgresGrantConditionFromStatus(grant, string(created.Status), "Created")
	if updateErr := h.updateStatusWithRetry(ctx, grant); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	log.Info("created PostgreSQL grant in Thalassa", "identity", created.Identity)
	return ctrl.Result{RequeueAfter: requeueAfterFromResourceStatus(grant.Status.ResourceStatus)}, nil
}

func (h *Handler) reconcilePostgresGrant(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	grant := in.Grant
	clusterIdentity := in.ClusterIdentity
	identity := grant.Status.ResourceID

	read := grant.Spec.Read
	write := grant.Spec.Write
	updated, err := h.DbaasClient.UpdatePgGrant(ctx, clusterIdentity, identity, dbaas.UpdatePgGrantRequest{
		Read:  &read,
		Write: &write,
	})
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			stdconditions.SetStandardConditions(&grant.Status.Conditions, stdconditions.ConditionStateDegraded, "NotFound", "Grant not found in Thalassa")
			grant.Status.ResourceID = ""
			grant.Status.ResourceStatus = ""
			if updateErr := h.updateStatusWithRetry(ctx, grant); updateErr != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		return h.setPostgresGrantErrorCondition(ctx, grant, "FailedUpdate", err.Error(), err)
	}
	h.Recorder.Eventf(grant, corev1.EventTypeNormal, "Updated", "Updated PostgreSQL grant in Thalassa (%s)", identity)
	grant.Status.ResourceStatus = string(updated.Status)
	grant.Status.LastReconcileError = ""
	setPostgresGrantConditionFromStatus(grant, string(updated.Status), "Synced")
	if updateErr := h.updateStatusWithRetry(ctx, grant); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	log.Info("updated PostgreSQL grant in Thalassa", "identity", identity)
	return ctrl.Result{RequeueAfter: requeueAfterFromResourceStatus(grant.Status.ResourceStatus)}, nil
}

func specToCreateGrantRequest(grant *dbaasv1.PostgresGrant) dbaas.CreatePgGrantRequest {
	return dbaas.CreatePgGrantRequest{
		Name:         grant.Spec.Name,
		RoleName:     grant.Spec.RoleName,
		DatabaseName: grant.Spec.DatabaseName,
		Read:         grant.Spec.Read,
		Write:        grant.Spec.Write,
	}
}

func requeueAfterFromResourceStatus(resourceStatus string) time.Duration {
	if strings.EqualFold(resourceStatus, stdconditions.ResourceStatusReady) || strings.TrimSpace(resourceStatus) == "" {
		return 5 * time.Minute
	}
	return 30 * time.Second
}
