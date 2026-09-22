package postgrescluster

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
	thalassahelpers "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/helpers"
	pgref "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresclusterref"
)

const (
	// restoreBackupUIDLabel identifies the on-demand backup taken for one PostgresCluster.
	// Label keys must be RFC 1123 (lowercase, digits, and hyphens).
	restoreBackupUIDLabel = "restore-for-uid"
)

var (
	errRestoreImmutable = errors.New("spec.restore is immutable after the cluster has been created")
	lsnPattern          = regexp.MustCompile(`^[0-9A-Fa-f]+/[0-9A-Fa-f]+$`)
)

// dbBackupAPI is the Thalassa backup surface used to restore a cluster.
// The source cluster is only read and backed up; it is never updated or deleted.
type dbBackupAPI interface {
	CreateDbBackup(ctx context.Context, dbClusterIdentity string, create dbaas.CreateDbClusterBackupRequest) (*dbaas.DbClusterBackup, error)
	GetDbBackup(ctx context.Context, backupIdentity string) (*dbaas.DbClusterBackup, error)
	ListDbBackupsForDbCluster(ctx context.Context, dbClusterIdentity string, listRequest *dbaas.ListDbBackupsRequest) ([]dbaas.DbClusterBackup, error)
}

type restoreWait struct {
	reason  string
	message string
	after   time.Duration
}

func validatePostgresClusterForRestore(pg *dbaasv1.PostgresCluster) error {
	if pg == nil {
		return fmt.Errorf("postgres cluster is required")
	}
	if err := validateRestoreSpec(pg.Spec.Restore); err != nil {
		return err
	}
	if pg.Spec.Restore != nil && pg.Spec.InitDb != nil {
		return fmt.Errorf("spec.initDb cannot be set when spec.restore is set")
	}
	return nil
}

func validateRestoreSpec(in *dbaasv1.PostgresClusterRestoreSpec) error {
	if in == nil {
		return nil
	}
	if strings.TrimSpace(in.BackupIdentity) == "" && !restoreSourceRefSet(in.SourceClusterRef) {
		return fmt.Errorf("spec.restore requires backupIdentity or sourceClusterRef name or identity")
	}
	if in.RecoveryTarget == nil {
		return nil
	}
	return validateRecoveryTarget(in.RecoveryTarget)
}

func restoreSourceRefSet(ref *dbaasv1.PostgresClusterRef) bool {
	if ref == nil {
		return false
	}
	return strings.TrimSpace(ref.Name) != "" || strings.TrimSpace(ref.Identity) != ""
}

func validateRecoveryTarget(in *dbaasv1.PostgresClusterRecoveryTarget) error {
	hasTime := in.TargetTime != nil && !in.TargetTime.IsZero()
	hasLSN := strings.TrimSpace(in.TargetLSN) != ""
	if hasTime == hasLSN {
		return fmt.Errorf("spec.restore.recoveryTarget requires exactly one of targetTime or targetLSN")
	}
	if hasLSN && !lsnPattern.MatchString(strings.TrimSpace(in.TargetLSN)) {
		return fmt.Errorf("spec.restore.recoveryTarget.targetLSN must be a PostgreSQL LSN")
	}
	return nil
}

func checkRestoreImmutable(pg *dbaasv1.PostgresCluster) error {
	if pg == nil {
		return fmt.Errorf("postgres cluster is required")
	}
	if pg.Status.ResourceID != "" && !restoreSpecMatchesApplied(pg) {
		return errRestoreImmutable
	}
	if pg.Status.AppliedRestore != nil && !equality.Semantic.DeepEqual(pg.Spec.Restore, pg.Status.AppliedRestore) {
		return errRestoreImmutable
	}
	if pg.Spec.Restore == nil && hasRestoredFromBackup(pg) {
		return errRestoreImmutable
	}
	return nil
}

func restoreSpecMatchesApplied(pg *dbaasv1.PostgresCluster) bool {
	if pg.Status.AppliedRestore == nil {
		return pg.Spec.Restore == nil
	}
	return equality.Semantic.DeepEqual(pg.Spec.Restore, pg.Status.AppliedRestore)
}

func effectiveRestoreSpec(pg *dbaasv1.PostgresCluster) *dbaasv1.PostgresClusterRestoreSpec {
	if pg != nil && pg.Status.AppliedRestore != nil {
		return pg.Status.AppliedRestore
	}
	if pg == nil {
		return nil
	}
	return pg.Spec.Restore
}

func hasRestoredFromBackup(pg *dbaasv1.PostgresCluster) bool {
	return pg != nil && pg.Status.RestoredFrom != nil && pg.Status.RestoredFrom.BackupIdentity != ""
}

func clusterRestoresFromBackup(pg *dbaasv1.PostgresCluster) bool {
	if pg == nil {
		return false
	}
	if pg.Spec.Restore != nil {
		return true
	}
	return hasRestoredFromBackup(pg)
}

func restoreAwaitingEndpoint(pg *dbaasv1.PostgresCluster) bool {
	if !clusterRestoresFromBackup(pg) {
		return false
	}
	return pg.Status.EndpointHost == "" || pg.Status.Port <= 0
}

func applyRestoreFields(req *dbaas.CreateDbClusterRequest, restore *dbaasv1.PostgresClusterRestoreSpec, backupID string) {
	if req == nil || backupID == "" {
		return
	}
	id := backupID
	req.RestoreFromBackupIdentity = &id
	if restore != nil {
		req.RestoreRecoveryTarget = recoveryTargetToAPI(restore.RecoveryTarget)
	}
}

func recoveryTargetToAPI(in *dbaasv1.PostgresClusterRecoveryTarget) *dbaas.RestoreRecoveryTarget {
	if in == nil {
		return nil
	}
	out := &dbaas.RestoreRecoveryTarget{}
	if in.TargetTime != nil && !in.TargetTime.IsZero() {
		formatted := in.TargetTime.UTC().Format(time.RFC3339)
		out.TargetTime = &formatted
	}
	if lsn := strings.TrimSpace(in.TargetLSN); lsn != "" {
		out.TargetLSN = &lsn
	}
	if out.TargetTime == nil && out.TargetLSN == nil {
		return nil
	}
	return out
}

func backupRestoreReadiness(status dbaas.ObjectStatus) (bool, bool) {
	switch strings.ToLower(string(status)) {
	case "completed", string(dbaas.ObjectStatusReady):
		return true, false
	case string(dbaas.ObjectStatusFailed), string(dbaas.ObjectStatusDeleted):
		return false, true
	default:
		return false, false
	}
}

func restoreWaitResult(err error, pending bool) (restoreWait, bool) {
	if errors.Is(err, pgref.ErrDependencyNotReady) {
		return restoreWait{
			reason:  "RestoreSourceNotReady",
			message: "Waiting for the source PostgreSQL cluster",
			after:   thalassahelpers.RequeueAfterDependencyNotReady,
		}, true
	}
	if err == nil && pending {
		return restoreWait{
			reason:  "RestoreBackupPending",
			message: "Waiting for the restore backup to complete",
			after:   requeueAfterRestoreBackupPending,
		}, true
	}
	return restoreWait{}, false
}

func (h *Handler) prepareClusterRestore(ctx context.Context, pg *dbaasv1.PostgresCluster) (string, *ctrl.Result, error) {
	if pg.Spec.Restore == nil && pg.Status.AppliedRestore == nil && !hasRestoredFromBackup(pg) {
		return "", nil, nil
	}
	backupID, pending, err := h.resolveRestoreBackup(ctx, h.DbaasClient, pg)
	if wait, ok := restoreWaitResult(err, pending); ok {
		stdconditions.SetStandardConditions(&pg.Status.Conditions, stdconditions.ConditionStateProgressing, wait.reason, wait.message)
		if updateErr := h.updateStatusWithRetry(ctx, pg); updateErr != nil {
			return "", nil, updateErr
		}
		res := ctrl.Result{RequeueAfter: wait.after}
		return "", &res, nil
	}
	if err != nil {
		return "", nil, err
	}
	return backupID, nil, nil
}

func (h *Handler) resolveRestoreBackup(ctx context.Context, api dbBackupAPI, pg *dbaasv1.PostgresCluster) (string, bool, error) {
	if err := checkRestoreImmutable(pg); err != nil {
		return "", false, err
	}
	if pg.Spec.Restore == nil {
		return "", false, nil
	}
	if err := validateRestoreSpec(pg.Spec.Restore); err != nil {
		return "", false, err
	}
	if err := h.captureAppliedRestore(ctx, pg); err != nil {
		return "", false, err
	}
	if api == nil {
		return "", false, fmt.Errorf("dbaas client is not configured")
	}

	spec := effectiveRestoreSpec(pg)
	if id := strings.TrimSpace(spec.BackupIdentity); id != "" {
		return h.waitForRestoreBackup(ctx, api, pg, id, "")
	}
	if hasRestoredFromBackup(pg) {
		return h.waitForRestoreBackup(ctx, api, pg, pg.Status.RestoredFrom.BackupIdentity, pg.Status.RestoredFrom.SourceClusterIdentity)
	}

	sourceID, err := h.resolveRestoreSource(ctx, pg)
	if err != nil {
		return "", false, err
	}
	existing, err := findRestoreBackup(ctx, api, sourceID, pg.UID)
	if err != nil {
		return "", false, err
	}
	if existing != nil {
		return h.waitForRestoreBackup(ctx, api, pg, existing.Identity, sourceID)
	}
	created, err := h.createRestoreBackup(ctx, api, pg, sourceID)
	if err != nil {
		return "", false, err
	}
	return h.waitForRestoreBackup(ctx, api, pg, created.Identity, sourceID)
}

func (h *Handler) captureAppliedRestore(ctx context.Context, pg *dbaasv1.PostgresCluster) error {
	if pg.Status.AppliedRestore != nil || pg.Spec.Restore == nil {
		return nil
	}
	if pg.Status.ResourceID != "" {
		return errRestoreImmutable
	}
	pg.Status.AppliedRestore = pg.Spec.Restore.DeepCopy()
	return h.updateStatusWithRetry(ctx, pg)
}

func (h *Handler) resolveRestoreSource(ctx context.Context, pg *dbaasv1.PostgresCluster) (string, error) {
	spec := effectiveRestoreSpec(pg)
	if spec == nil || spec.SourceClusterRef == nil {
		return "", fmt.Errorf("spec.restore.sourceClusterRef is required when backupIdentity is empty")
	}
	return pgref.Resolve(ctx, h.Client, pg.Namespace, *spec.SourceClusterRef)
}

func (h *Handler) waitForRestoreBackup(ctx context.Context, api dbBackupAPI, pg *dbaasv1.PostgresCluster, backupID, sourceID string) (string, bool, error) {
	if backupID == "" {
		return "", false, fmt.Errorf("restore backup identity is empty")
	}
	if err := h.persistRestoredFrom(ctx, pg, backupID, sourceID); err != nil {
		return "", false, err
	}
	backup, err := api.GetDbBackup(ctx, backupID)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			return "", false, fmt.Errorf("restore backup %s not found", backupID)
		}
		return "", false, fmt.Errorf("get restore backup: %w", err)
	}
	if backup == nil || backup.Identity == "" {
		return "", false, fmt.Errorf("restore backup %s not found", backupID)
	}
	observedSource := sourceID
	if backup.DbCluster != nil && backup.DbCluster.Identity != "" {
		observedSource = backup.DbCluster.Identity
	}
	if err := h.persistRestoredFrom(ctx, pg, backup.Identity, observedSource); err != nil {
		return "", false, err
	}
	ready, failed := backupRestoreReadiness(backup.Status)
	if failed {
		msg := backup.StatusMessage
		if msg == "" {
			msg = string(backup.Status)
		}
		return "", false, fmt.Errorf("restore backup %s failed: %s", backupID, msg)
	}
	if !ready {
		return backupID, true, nil
	}
	return backupID, false, nil
}

func (h *Handler) persistRestoredFrom(ctx context.Context, pg *dbaasv1.PostgresCluster, backupID, sourceID string) error {
	if !recordRestoredFrom(pg, backupID, sourceID) {
		return nil
	}
	return h.updateStatusWithRetry(ctx, pg)
}

func recordRestoredFrom(pg *dbaasv1.PostgresCluster, backupID, sourceID string) bool {
	if backupID == "" && sourceID == "" {
		return false
	}
	if pg.Status.RestoredFrom == nil {
		pg.Status.RestoredFrom = &dbaasv1.PostgresClusterRestoredFromStatus{}
	}
	changed := false
	if backupID != "" && pg.Status.RestoredFrom.BackupIdentity != backupID {
		pg.Status.RestoredFrom.BackupIdentity = backupID
		changed = true
	}
	if sourceID != "" && pg.Status.RestoredFrom.SourceClusterIdentity != sourceID {
		pg.Status.RestoredFrom.SourceClusterIdentity = sourceID
		changed = true
	}
	return changed
}

func (h *Handler) createRestoreBackup(ctx context.Context, api dbBackupAPI, pg *dbaasv1.PostgresCluster, sourceID string) (*dbaas.DbClusterBackup, error) {
	if pg.UID == "" {
		return nil, fmt.Errorf("postgres cluster UID is required to take a restore backup")
	}
	desc := fmt.Sprintf("On-demand backup taken to restore PostgreSQL cluster %s/%s", pg.Namespace, pg.Name)
	created, err := api.CreateDbBackup(ctx, sourceID, dbaas.CreateDbClusterBackupRequest{
		Name:        restoreBackupName(pg.UID),
		Description: &desc,
		Labels: dbaas.Labels{
			restoreBackupUIDLabel: strings.ToLower(string(pg.UID)),
		},
		Annotations: dbaas.Annotations{
			"managed-by": "thalassa-dbaas-manager",
		},
	})
	if err != nil {
		existing, findErr := findRestoreBackup(ctx, api, sourceID, pg.UID)
		if findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, fmt.Errorf("create restore backup: %w", err)
	}
	if created == nil || created.Identity == "" {
		return nil, fmt.Errorf("create restore backup: empty backup identity")
	}
	if h.Recorder != nil {
		h.Recorder.Eventf(pg, corev1.EventTypeNormal, "RestoreBackupCreated", "Created on-demand backup %s of cluster %s", created.Identity, sourceID)
	}
	return created, nil
}

func findRestoreBackup(ctx context.Context, api dbBackupAPI, sourceID string, uid types.UID) (*dbaas.DbClusterBackup, error) {
	backups, err := api.ListDbBackupsForDbCluster(ctx, sourceID, nil)
	if err != nil {
		return nil, fmt.Errorf("list backups for cluster %s: %w", sourceID, err)
	}
	want := strings.ToLower(string(uid))
	for i := range backups {
		if backupMatchesRestore(backups[i], want) {
			found := backups[i]
			return &found, nil
		}
	}
	return nil, nil
}

func backupMatchesRestore(backup dbaas.DbClusterBackup, uid string) bool {
	if uid == "" || backup.Labels == nil {
		return false
	}
	return strings.EqualFold(backup.Labels[restoreBackupUIDLabel], uid)
}

func restoreBackupName(uid types.UID) string {
	return "restore-" + strings.ToLower(string(uid))
}
