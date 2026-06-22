package postgresrole

import (
	"context"
	"errors"
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

func (h *Handler) createPostgresRole(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	role := in.Role
	clusterIdentity := in.ClusterIdentity
	password := in.Password

	stdconditions.SetStandardConditions(&role.Status.Conditions, stdconditions.ConditionStateProgressing, "Creating", "Creating PostgreSQL role in Thalassa")
	if updateErr := h.updateStatusWithRetry(ctx, role); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	if password != "" && role.Spec.WriteConnectionSecretToRef != nil {
		if err := h.writeConnectionSecretCredentials(ctx, role, password); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
		}
	}
	created, adopted, err := h.createOrAdoptPgRole(ctx, clusterIdentity, role, password)
	if err != nil {
		return h.setPostgresRoleErrorCondition(ctx, role, "FailedCreate", err.Error())
	}
	eventReason := "Created"
	if adopted {
		eventReason = "Adopted"
		if password != "" {
			updated, updateErr := h.DbaasClient.UpdatePgRole(ctx, clusterIdentity, created.Identity, specToUpdateRoleRequest(role, password))
			if updateErr != nil {
				return h.setPostgresRoleErrorCondition(ctx, role, "FailedUpdate", updateErr.Error())
			}
			created = updated
		}
	}
	h.Recorder.Eventf(role, corev1.EventTypeNormal, eventReason, "%s PostgreSQL role in Thalassa (%s)", eventReason, created.Identity)
	role.Status.ResourceID = created.Identity
	role.Status.ResourceStatus = string(created.Status)
	role.Status.LastReconcileError = ""
	h.setPostgresRoleConditionFromStatus(role, string(created.Status), eventReason)
	if updateErr := h.updateStatusWithRetry(ctx, role); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	if adopted {
		log.Info("adopted existing PostgreSQL role in Thalassa", "identity", created.Identity)
	} else {
		log.Info("created PostgreSQL role in Thalassa", "identity", created.Identity)
	}
	if result, err := h.reconcileConnectionSecretOrRequeue(ctx, role, clusterIdentity, password); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	requeueAfter := 5 * time.Minute
	if !isPostgresRoleStatusReady(role.Status.ResourceStatus) {
		requeueAfter = 30 * time.Second
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (h *Handler) reconcilePostgresRole(ctx context.Context, in ReconcileInput) (ctrl.Result, error) {
	role := in.Role
	clusterIdentity := in.ClusterIdentity
	password := in.Password
	identity := role.Status.ResourceID
	updated, err := h.DbaasClient.UpdatePgRole(ctx, clusterIdentity, identity, specToUpdateRoleRequest(role, password))
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			stdconditions.SetStandardConditions(&role.Status.Conditions, stdconditions.ConditionStateDegraded, "NotFound", "Role not found in Thalassa")
			role.Status.ResourceID = ""
			role.Status.ResourceStatus = ""
			if updateErr := h.updateStatusWithRetry(ctx, role); updateErr != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		return h.setPostgresRoleErrorCondition(ctx, role, "FailedUpdate", err.Error())
	}
	h.Recorder.Eventf(role, corev1.EventTypeNormal, "Updated", "Updated PostgreSQL role in Thalassa (%s)", identity)
	role.Status.ResourceStatus = string(updated.Status)
	role.Status.LastReconcileError = ""
	h.setPostgresRoleConditionFromStatus(role, string(updated.Status), "Synced")
	if updateErr := h.updateStatusWithRetry(ctx, role); updateErr != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, updateErr
	}
	if result, err := h.reconcileConnectionSecretOrRequeue(ctx, role, clusterIdentity, password); err != nil || result.RequeueAfter > 0 {
		return result, err
	}
	requeueAfter := 5 * time.Minute
	if !isPostgresRoleStatusReady(role.Status.ResourceStatus) {
		requeueAfter = 30 * time.Second
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (h *Handler) reconcileConnectionSecretOrRequeue(ctx context.Context, role *dbaasv1.PostgresRole, clusterIdentity, password string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if err := h.reconcileConnectionSecret(ctx, role, clusterIdentity, password); err != nil {
		if errors.Is(err, ErrEndpointNotReady) {
			log.Info("waiting for PostgresCluster endpoint before writing connection secret")
			return ctrl.Result{RequeueAfter: requeueAfterEndpointNotReady}, nil
		}
		log.Error(err, "failed to reconcile connection secret, will retry")
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, nil
	}
	return ctrl.Result{}, nil
}

func isPostgresRoleStatusReady(resourceStatus string) bool {
	return strings.EqualFold(resourceStatus, stdconditions.ResourceStatusReady) || strings.TrimSpace(resourceStatus) == ""
}

func specToCreateRoleRequest(role *dbaasv1.PostgresRole, password string) dbaas.CreatePgRoleRequest {
	req := dbaas.CreatePgRoleRequest{
		Name:       role.Spec.Name,
		Login:      role.Spec.Login,
		CreateDb:   role.Spec.CreateDb,
		CreateRole: role.Spec.CreateRole,
		Password:   password,
	}
	if role.Spec.ConnectionLimit != nil {
		req.ConnectionLimit = *role.Spec.ConnectionLimit
	}
	if role.Spec.ValidUntil != nil {
		req.ValidUntil = &role.Spec.ValidUntil.Time
	}
	return req
}

func specToUpdateRoleRequest(role *dbaasv1.PostgresRole, password string) dbaas.UpdatePgRoleRequest {
	req := dbaas.UpdatePgRoleRequest{}
	if role.Spec.ConnectionLimit != nil {
		req.ConnectionLimit = *role.Spec.ConnectionLimit
	}
	if role.Spec.ValidUntil != nil {
		req.ValidUntil = &role.Spec.ValidUntil.Time
	}
	if password != "" {
		req.Password = &password
	}
	return req
}

// setPostgresRoleConditionFromStatus sets Available/Ready from the role resource status (DBaaS ObjectStatus).
// Empty status is treated as ready for now (API may omit status when ready).
func (h *Handler) setPostgresRoleConditionFromStatus(role *dbaasv1.PostgresRole, resourceStatus string, reason string) {
	switch {
	case isPostgresRoleStatusReady(resourceStatus):
		stdconditions.SetStandardConditions(&role.Status.Conditions, stdconditions.ConditionStateAvailable, reason, "PostgreSQL role is ready")
	case strings.EqualFold(resourceStatus, string(dbaas.ObjectStatusFailed)),
		strings.EqualFold(resourceStatus, string(dbaas.ObjectStatusDeleted)):
		stdconditions.SetStandardConditions(&role.Status.Conditions, stdconditions.ConditionStateDegraded, "RoleNotReady", "Role status: "+resourceStatus)
	default:
		stdconditions.SetStandardConditions(&role.Status.Conditions, stdconditions.ConditionStateProgressing, "RoleNotReady", "Role status: "+resourceStatus)
	}
}
