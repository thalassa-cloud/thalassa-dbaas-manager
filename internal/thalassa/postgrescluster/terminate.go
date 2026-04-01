package postgrescluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func effectivePostgresClusterOnDelete(p dbaasv1.PostgresClusterOnDeletePolicy) dbaasv1.PostgresClusterOnDeletePolicy {
	if p == "" {
		return dbaasv1.PostgresClusterOnDeleteDelete
	}
	return p
}

func (h *Handler) terminate(ctx context.Context, pg *dbaasv1.PostgresCluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(pg, Finalizer) {
		return ctrl.Result{}, nil
	}

	policy := effectivePostgresClusterOnDelete(pg.Spec.OnDelete)
	identity := pg.Status.ResourceID

	switch policy {
	case dbaasv1.PostgresClusterOnDeleteOrphan:
		h.Recorder.Event(pg, corev1.EventTypeNormal, "Orphaned", "Leaving PostgreSQL cluster running in Thalassa (onDelete=Orphan)")
		if err := h.removePostgresClusterAnnotation(ctx, pg, PreDeleteBackupIdentityAnnotation); err != nil {
			return ctrl.Result{}, err
		}

	case dbaasv1.PostgresClusterOnDeleteDelete:
		if identity != "" {
			if err := h.DbaasClient.DeleteDbCluster(ctx, identity); err != nil && !thalassaclient.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			log.Info("deleted PostgreSQL cluster in Thalassa", "identity", identity)
			h.Recorder.Eventf(pg, corev1.EventTypeNormal, "Deleted", "Deleted PostgreSQL cluster in Thalassa (%s)", identity)
		}
		if err := h.removePostgresClusterAnnotation(ctx, pg, PreDeleteBackupIdentityAnnotation); err != nil {
			return ctrl.Result{}, err
		}

	case dbaasv1.PostgresClusterOnDeleteBackupAndDelete:
		if identity == "" {
			if err := h.removePostgresClusterAnnotation(ctx, pg, PreDeleteBackupIdentityAnnotation); err != nil {
				return ctrl.Result{}, err
			}
			break
		}
		extra, err := h.reconcileDeleteBackupAndDelete(ctx, pg, identity)
		if err != nil {
			return ctrl.Result{}, err
		}
		if extra != nil {
			return *extra, nil
		}
	default:
		if identity != "" {
			if err := h.DbaasClient.DeleteDbCluster(ctx, identity); err != nil && !thalassaclient.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			log.Info("deleted PostgreSQL cluster in Thalassa", "identity", identity)
			h.Recorder.Eventf(pg, corev1.EventTypeNormal, "Deleted", "Deleted PostgreSQL cluster in Thalassa (%s)", identity)
		}
		if err := h.removePostgresClusterAnnotation(ctx, pg, PreDeleteBackupIdentityAnnotation); err != nil {
			return ctrl.Result{}, err
		}
	}

	var latest dbaasv1.PostgresCluster
	if err := h.Client.Get(ctx, client.ObjectKeyFromObject(pg), &latest); err != nil {
		return ctrl.Result{}, err
	}
	if !controllerutil.ContainsFinalizer(&latest, Finalizer) {
		return ctrl.Result{}, nil
	}
	if controllerutil.RemoveFinalizer(&latest, Finalizer) {
		if err := h.Client.Update(ctx, &latest); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (h *Handler) reconcileDeleteBackupAndDelete(ctx context.Context, pg *dbaasv1.PostgresCluster, clusterIdentity string) (*ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if _, err := h.DbaasClient.GetDbCluster(ctx, clusterIdentity); err != nil && thalassaclient.IsNotFound(err) {
		return nil, nil
	}

	key := PreDeleteBackupIdentityAnnotation
	backupID := ""
	if pg.Annotations != nil {
		backupID = pg.Annotations[key]
	}

	if backupID == "" {
		name := fmt.Sprintf("pre-delete-%s-%d", pg.UID, time.Now().Unix())
		created, err := h.DbaasClient.CreateDbBackup(ctx, clusterIdentity, dbaas.CreateDbClusterBackupRequest{
			Name:   name,
			Labels: dbaas.Labels{},
			Annotations: dbaas.Annotations{
				"description": fmt.Sprintf("Pre-delete backup for PostgreSQL cluster deletion (%s)", pg.Name),
				"managed-by":  "thalassa-dbaas-manager",
			},
		})
		if err != nil {
			return nil, fmt.Errorf("create pre-delete backup: %w", err)
		}
		h.Recorder.Eventf(pg, corev1.EventTypeNormal, "PreDeleteBackupCreated", "Created pre-delete backup %q (%s)", name, created.Identity)
		if err := h.setPostgresClusterAnnotation(ctx, pg, key, created.Identity); err != nil {
			return nil, err
		}
		return &ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	b, err := h.DbaasClient.GetDbBackup(ctx, backupID)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			if err := h.removePostgresClusterAnnotation(ctx, pg, key); err != nil {
				return nil, err
			}
			return &ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		return nil, fmt.Errorf("get pre-delete backup: %w", err)
	}

	switch strings.ToLower(string(b.Status)) {
	case "completed":
		if err := h.DbaasClient.DeleteDbCluster(ctx, clusterIdentity); err != nil && !thalassaclient.IsNotFound(err) {
			return nil, err
		}
		log.Info("deleted PostgreSQL cluster in Thalassa after final backup", "identity", clusterIdentity)
		h.Recorder.Eventf(pg, corev1.EventTypeNormal, "Deleted", "Deleted PostgreSQL cluster in Thalassa after final backup (%s)", clusterIdentity)
		return nil, nil
	case "failed":
		h.Recorder.Eventf(pg, corev1.EventTypeWarning, "PreDeleteBackupFailed", "Pre-delete backup %s failed: %s", backupID, b.StatusMessage)
		return nil, fmt.Errorf("pre-delete backup failed: %s", b.StatusMessage)
	default:
		msg := b.StatusMessage
		if msg == "" {
			msg = fmt.Sprintf("backup status: %s", b.Status)
		}
		h.Recorder.Eventf(pg, corev1.EventTypeNormal, "PreDeleteBackupPending", "Waiting for pre-delete backup %s to complete (%s)", backupID, msg)

		return &ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
}
