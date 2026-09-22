package postgrescluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	pgref "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresclusterref"
)

func TestValidateRestoreSpec(t *testing.T) {
	t.Parallel()

	when := metav1.NewTime(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	tests := []struct {
		name    string
		pg      *dbaasv1.PostgresCluster
		wantErr string
	}{
		{name: "nil restore", pg: &dbaasv1.PostgresCluster{}},
		{
			name: "backup identity",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				BackupIdentity: "backup-1",
			}}},
		},
		{
			name: "source name",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "source"},
			}}},
		},
		{
			name: "source identity",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				SourceClusterRef: &dbaasv1.PostgresClusterRef{Identity: "db-source"},
			}}},
		},
		{
			name: "target time",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				BackupIdentity: "backup-1",
				RecoveryTarget: &dbaasv1.PostgresClusterRecoveryTarget{TargetTime: &when},
			}}},
		},
		{
			name: "target lsn",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				BackupIdentity: "backup-1",
				RecoveryTarget: &dbaasv1.PostgresClusterRecoveryTarget{TargetLSN: "0/16B3748"},
			}}},
		},
		{
			name:    "missing source and backup",
			pg:      &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{}}},
			wantErr: "backupIdentity or sourceClusterRef",
		},
		{
			name: "empty source ref",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				SourceClusterRef: &dbaasv1.PostgresClusterRef{},
			}}},
			wantErr: "backupIdentity or sourceClusterRef",
		},
		{
			name: "both recovery targets",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				BackupIdentity: "backup-1",
				RecoveryTarget: &dbaasv1.PostgresClusterRecoveryTarget{TargetTime: &when, TargetLSN: "0/1"},
			}}},
			wantErr: "exactly one",
		},
		{
			name: "empty recovery target",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				BackupIdentity: "backup-1",
				RecoveryTarget: &dbaasv1.PostgresClusterRecoveryTarget{},
			}}},
			wantErr: "exactly one",
		},
		{
			name: "bad lsn",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
				BackupIdentity: "backup-1",
				RecoveryTarget: &dbaasv1.PostgresClusterRecoveryTarget{TargetLSN: "not-an-lsn"},
			}}},
			wantErr: "targetLSN",
		},
		{
			name: "initdb with restore",
			pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{
				Restore: &dbaasv1.PostgresClusterRestoreSpec{BackupIdentity: "backup-1"},
				InitDb:  &dbaasv1.PostgresInitDbSpec{Encoding: "UTF8"},
			}},
			wantErr: "initDb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePostgresClusterForRestore(tt.pg)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCheckRestoreImmutable(t *testing.T) {
	t.Parallel()

	restore := &dbaasv1.PostgresClusterRestoreSpec{
		SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "source"},
	}
	changed := &dbaasv1.PostgresClusterRestoreSpec{
		SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "other"},
	}
	tests := []struct {
		name    string
		pg      *dbaasv1.PostgresCluster
		wantErr bool
	}{
		{name: "new cluster", pg: &dbaasv1.PostgresCluster{Spec: dbaasv1.PostgresClusterSpec{Restore: restore}}},
		{
			name: "created without restore",
			pg:   &dbaasv1.PostgresCluster{Status: dbaasv1.PostgresClusterStatus{ResourceID: "db-1"}},
		},
		{
			name: "matches applied restore before create",
			pg: &dbaasv1.PostgresCluster{
				Spec:   dbaasv1.PostgresClusterSpec{Restore: restore},
				Status: dbaasv1.PostgresClusterStatus{AppliedRestore: restore.DeepCopy()},
			},
		},
		{
			name: "spec changed after restore started",
			pg: &dbaasv1.PostgresCluster{
				Spec:   dbaasv1.PostgresClusterSpec{Restore: changed},
				Status: dbaasv1.PostgresClusterStatus{AppliedRestore: restore.DeepCopy()},
			},
			wantErr: true,
		},
		{
			name: "restore added after cluster create",
			pg: &dbaasv1.PostgresCluster{
				Spec:   dbaasv1.PostgresClusterSpec{Restore: restore},
				Status: dbaasv1.PostgresClusterStatus{ResourceID: "db-1"},
			},
			wantErr: true,
		},
		{
			name: "restore removed after backup was recorded",
			pg: &dbaasv1.PostgresCluster{
				Status: dbaasv1.PostgresClusterStatus{
					RestoredFrom: &dbaasv1.PostgresClusterRestoredFromStatus{BackupIdentity: "backup-1"},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkRestoreImmutable(tt.pg)
			if tt.wantErr {
				assert.ErrorIs(t, err, errRestoreImmutable)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestSpecToCreateRequestRestore(t *testing.T) {
	t.Parallel()

	when := metav1.NewTime(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	h := &Handler{}
	base := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "copy"},
		Spec: dbaasv1.PostgresClusterSpec{
			Description:       "copied",
			PostgresVersion:   "18",
			InstanceType:      dbaasv1.DbInstanceTypeRef{ID: "db-pgp-medium"},
			StorageGB:         15,
			VolumeTypeClassId: "block",
			Instances:         1,
			InitDb:            &dbaasv1.PostgresInitDbSpec{Encoding: "UTF8"},
			Restore: &dbaasv1.PostgresClusterRestoreSpec{
				SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "source"},
				RecoveryTarget:   &dbaasv1.PostgresClusterRecoveryTarget{TargetTime: &when},
			},
		},
	}

	withRestore := h.specToCreateRequest(base, "subnet", nil, "18", "", "backup-used")
	require.NotNil(t, withRestore.RestoreFromBackupIdentity)
	assert.Equal(t, "backup-used", *withRestore.RestoreFromBackupIdentity)
	require.NotNil(t, withRestore.RestoreRecoveryTarget)
	require.NotNil(t, withRestore.RestoreRecoveryTarget.TargetTime)
	assert.Equal(t, "2026-09-21T12:00:00Z", *withRestore.RestoreRecoveryTarget.TargetTime)
	assert.Nil(t, withRestore.PostgresInitDb)

	lsnPG := base.DeepCopy()
	lsnPG.Spec.Restore.RecoveryTarget = &dbaasv1.PostgresClusterRecoveryTarget{TargetLSN: "0/16B3748"}
	withLSN := h.specToCreateRequest(lsnPG, "subnet", nil, "18", "", "backup-used")
	require.NotNil(t, withLSN.RestoreRecoveryTarget)
	require.NotNil(t, withLSN.RestoreRecoveryTarget.TargetLSN)
	assert.Equal(t, "0/16B3748", *withLSN.RestoreRecoveryTarget.TargetLSN)
	assert.Nil(t, withLSN.RestoreRecoveryTarget.TargetTime)

	empty := base.DeepCopy()
	empty.Spec.Restore = nil
	without := h.specToCreateRequest(empty, "subnet", nil, "18", "", "")
	assert.Nil(t, without.RestoreFromBackupIdentity)
	assert.Nil(t, without.RestoreRecoveryTarget)
	require.NotNil(t, without.PostgresInitDb)
	assert.Equal(t, "UTF8", without.PostgresInitDb.Encoding)
}

func TestBackupRestoreReadiness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status     dbaas.ObjectStatus
		wantReady  bool
		wantFailed bool
	}{
		{status: "completed", wantReady: true},
		{status: dbaas.ObjectStatusReady, wantReady: true},
		{status: dbaas.ObjectStatusFailed, wantFailed: true},
		{status: dbaas.ObjectStatusDeleted, wantFailed: true},
		{status: dbaas.ObjectStatusCreating},
		{status: "running"},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			ready, failed := backupRestoreReadiness(tt.status)
			assert.Equal(t, tt.wantReady, ready)
			assert.Equal(t, tt.wantFailed, failed)
		})
	}
}

func TestRestoreAwaitingEndpoint(t *testing.T) {
	t.Parallel()

	h := &Handler{}
	tests := []struct {
		name      string
		pg        *dbaasv1.PostgresCluster
		wantReady bool
	}{
		{
			name: "restore waits for endpoint",
			pg: &dbaasv1.PostgresCluster{
				Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{BackupIdentity: "backup-1"}},
				Status: dbaasv1.PostgresClusterStatus{
					ReconcileStatus: dbaasv1.ReconcileStatus{ResourceStatus: "ready"},
					ReadyObservedAt: &metav1.Time{Time: time.Now().Add(-time.Minute)},
				},
			},
		},
		{
			name: "restore ready when endpoint is published",
			pg: &dbaasv1.PostgresCluster{
				Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{BackupIdentity: "backup-1"}},
				Status: dbaasv1.PostgresClusterStatus{
					ReconcileStatus: dbaasv1.ReconcileStatus{ResourceStatus: "ready"},
					EndpointHost:    "10.0.0.8",
					Port:            5432,
					ReadyObservedAt: &metav1.Time{Time: time.Now().Add(-time.Minute)},
				},
			},
			wantReady: true,
		},
		{
			name: "empty cluster can be ready without an endpoint check",
			pg: &dbaasv1.PostgresCluster{
				Status: dbaasv1.PostgresClusterStatus{
					ReconcileStatus: dbaasv1.ReconcileStatus{ResourceStatus: "ready"},
					ReadyObservedAt: &metav1.Time{Time: time.Now().Add(-time.Minute)},
				},
			},
			wantReady: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h.setPostgresClusterConditionWithStability(tt.pg)
			assert.Equal(t, tt.wantReady, meta.IsStatusConditionTrue(tt.pg.Status.Conditions, "Ready"))
			if tt.wantReady {
				return
			}
			cond := meta.FindStatusCondition(tt.pg.Status.Conditions, "Ready")
			require.NotNil(t, cond)
			assert.Equal(t, "EndpointNotReady", cond.Reason)
		})
	}
}

func TestResolveRestoreBackup(t *testing.T) {
	t.Parallel()

	const (
		ns     = "db"
		source = "source"
		copy   = "copy"
		uid    = types.UID("11111111-1111-1111-1111-111111111111")
	)

	sourceCluster := func(resourceID string) *dbaasv1.PostgresCluster {
		return &dbaasv1.PostgresCluster{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: ns},
			Spec:       dbaasv1.PostgresClusterSpec{DeleteProtection: true, PostgresVersion: "18"},
			Status:     dbaasv1.PostgresClusterStatus{ResourceID: resourceID},
		}
	}
	candidate := func(restore *dbaasv1.PostgresClusterRestoreSpec) *dbaasv1.PostgresCluster {
		return &dbaasv1.PostgresCluster{
			ObjectMeta: metav1.ObjectMeta{Name: copy, Namespace: ns, UID: uid},
			Spec:       dbaasv1.PostgresClusterSpec{Restore: restore, PostgresVersion: "18"},
		}
	}

	t.Run("takes a new backup and does not reuse a scheduled one", func(t *testing.T) {
		t.Parallel()
		src := sourceCluster("db-source")
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{
			SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: source},
		})
		api := &fakeBackupAPI{backups: map[string]*dbaas.DbClusterBackup{
			"backup-old": {
				Identity:  "backup-old",
				Status:    "completed",
				DbCluster: &dbaas.DbCluster{Identity: "db-source"},
			},
		}}
		h := newRestoreHandler(t, src, pg)

		id, pending, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)
		assert.True(t, pending)
		assert.NotEqual(t, "backup-old", id)
		assert.Equal(t, []string{"db-source"}, api.createFor)
		assert.Equal(t, id, pg.Status.RestoredFrom.BackupIdentity)
		assert.Equal(t, "db-source", pg.Status.RestoredFrom.SourceClusterIdentity)
		require.NotNil(t, pg.Status.AppliedRestore)
		assert.Empty(t, pg.Status.AppliedRestore.BackupIdentity)
		assertSourceUnchanged(t, h.Client, src)

		api.backups[id].Status = "completed"
		id2, pending, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)
		assert.False(t, pending)
		assert.Equal(t, id, id2)
		assert.Equal(t, 1, api.creates)
		assertSourceUnchanged(t, h.Client, src)
	})

	t.Run("uses a caller supplied backup without creating one", func(t *testing.T) {
		t.Parallel()
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{BackupIdentity: "backup-given"})
		api := &fakeBackupAPI{backups: map[string]*dbaas.DbClusterBackup{
			"backup-given": {
				Identity:  "backup-given",
				Status:    dbaas.ObjectStatusReady,
				DbCluster: &dbaas.DbCluster{Identity: "db-source"},
			},
		}}
		h := newRestoreHandler(t, pg)

		id, pending, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)
		assert.False(t, pending)
		assert.Equal(t, "backup-given", id)
		assert.Empty(t, api.createFor)
		assert.Equal(t, "db-source", pg.Status.RestoredFrom.SourceClusterIdentity)
	})

	t.Run("waits when the source cluster has no identity", func(t *testing.T) {
		t.Parallel()
		src := sourceCluster("")
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{
			SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: source},
		})
		api := &fakeBackupAPI{}
		h := newRestoreHandler(t, src, pg)

		_, _, err := h.resolveRestoreBackup(context.Background(), api, pg)
		assert.ErrorIs(t, err, pgref.ErrDependencyNotReady)
		assert.Zero(t, api.creates)
		assertSourceUnchanged(t, h.Client, src)
	})

	t.Run("accepts a source identity without a kubernetes object", func(t *testing.T) {
		t.Parallel()
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{
			SourceClusterRef: &dbaasv1.PostgresClusterRef{Identity: "db-direct"},
		})
		api := &fakeBackupAPI{}
		h := newRestoreHandler(t, pg)

		id, pending, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)
		assert.True(t, pending)
		assert.NotEmpty(t, id)
		assert.Equal(t, []string{"db-direct"}, api.createFor)
	})

	t.Run("does not take a second backup when the first failed", func(t *testing.T) {
		t.Parallel()
		src := sourceCluster("db-source")
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{
			SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: source},
		})
		api := &fakeBackupAPI{}
		h := newRestoreHandler(t, src, pg)

		id, _, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)
		api.backups[id].Status = dbaas.ObjectStatusFailed
		api.backups[id].StatusMessage = "upload failed"

		_, _, err = h.resolveRestoreBackup(context.Background(), api, pg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upload failed")
		assert.Equal(t, 1, api.creates)
		assert.Equal(t, id, pg.Status.RestoredFrom.BackupIdentity)
	})

	t.Run("rejects a restore spec change after a backup was recorded", func(t *testing.T) {
		t.Parallel()
		src := sourceCluster("db-source")
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{
			SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: source},
		})
		api := &fakeBackupAPI{}
		h := newRestoreHandler(t, src, pg)
		_, _, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)

		pg.Spec.Restore.SourceClusterRef.Name = "other"
		_, _, err = h.resolveRestoreBackup(context.Background(), api, pg)
		assert.ErrorIs(t, err, errRestoreImmutable)
		assert.Equal(t, 1, api.creates)
	})

	t.Run("reuses an on-demand backup created before status was saved", func(t *testing.T) {
		t.Parallel()
		src := sourceCluster("db-source")
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{
			SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: source},
		})
		existing := &dbaas.DbClusterBackup{
			Identity: "backup-existing",
			Status:   "completed",
			Labels:   dbaas.Labels{restoreBackupUIDLabel: string(uid)},
			DbCluster: &dbaas.DbCluster{
				Identity: "db-source",
			},
		}
		api := &fakeBackupAPI{
			createErr:         errors.New("already exists"),
			revealOnCreateErr: existing,
		}
		h := newRestoreHandler(t, src, pg)

		id, pending, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.NoError(t, err)
		assert.False(t, pending)
		assert.Equal(t, "backup-existing", id)
		assert.Equal(t, 1, api.creates)
	})

	t.Run("missing supplied backup is an error", func(t *testing.T) {
		t.Parallel()
		pg := candidate(&dbaasv1.PostgresClusterRestoreSpec{BackupIdentity: "backup-missing"})
		api := &fakeBackupAPI{getErr: thalassaclient.ErrNotFound}
		h := newRestoreHandler(t, pg)

		_, _, err := h.resolveRestoreBackup(context.Background(), api, pg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Zero(t, api.creates)
	})
}

func newRestoreHandler(t *testing.T, objects ...client.Object) *Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, dbaasv1.AddToScheme(scheme))
	copied := make([]client.Object, 0, len(objects))
	for _, obj := range objects {
		copied = append(copied, obj.DeepCopyObject().(client.Object))
	}
	return &Handler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&dbaasv1.PostgresCluster{}).WithObjects(copied...).Build(),
		Recorder: record.NewFakeRecorder(8),
	}
}

func assertSourceUnchanged(t *testing.T, c client.Client, want *dbaasv1.PostgresCluster) {
	t.Helper()
	var got dbaasv1.PostgresCluster
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(want), &got))
	assert.Equal(t, want.Spec, got.Spec)
	assert.Equal(t, want.Status.ResourceID, got.Status.ResourceID)
	assert.True(t, got.Spec.DeleteProtection)
}

type fakeBackupAPI struct {
	backups           map[string]*dbaas.DbClusterBackup
	creates           int
	createFor         []string
	getErr            error
	createErr         error
	revealOnCreateErr *dbaas.DbClusterBackup
}

func (f *fakeBackupAPI) CreateDbBackup(_ context.Context, dbClusterIdentity string, create dbaas.CreateDbClusterBackupRequest) (*dbaas.DbClusterBackup, error) {
	f.creates++
	f.createFor = append(f.createFor, dbClusterIdentity)
	if f.createErr != nil {
		if f.revealOnCreateErr != nil {
			if f.backups == nil {
				f.backups = map[string]*dbaas.DbClusterBackup{}
			}
			f.backups[f.revealOnCreateErr.Identity] = f.revealOnCreateErr
		}
		return nil, f.createErr
	}
	if create.Name == "" {
		return nil, errors.New("backup name is required")
	}
	if _, ok := create.Labels[restoreBackupUIDLabel]; !ok {
		return nil, errors.New("restore label is required")
	}
	identity := "backup-new-" + dbClusterIdentity
	backup := &dbaas.DbClusterBackup{
		Identity:  identity,
		Labels:    dbaas.Labels{restoreBackupUIDLabel: create.Labels[restoreBackupUIDLabel]},
		Status:    dbaas.ObjectStatusCreating,
		DbCluster: &dbaas.DbCluster{Identity: dbClusterIdentity},
	}
	if f.backups == nil {
		f.backups = map[string]*dbaas.DbClusterBackup{}
	}
	f.backups[identity] = backup
	return backup, nil
}

func (f *fakeBackupAPI) GetDbBackup(_ context.Context, backupIdentity string) (*dbaas.DbClusterBackup, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	backup, ok := f.backups[backupIdentity]
	if !ok {
		return nil, thalassaclient.ErrNotFound
	}
	return backup, nil
}

func (f *fakeBackupAPI) ListDbBackupsForDbCluster(_ context.Context, dbClusterIdentity string, _ *dbaas.ListDbBackupsRequest) ([]dbaas.DbClusterBackup, error) {
	var out []dbaas.DbClusterBackup
	for _, backup := range f.backups {
		if backup.DbCluster != nil && backup.DbCluster.Identity == dbClusterIdentity {
			out = append(out, *backup)
		}
	}
	return out, nil
}

var _ dbBackupAPI = (*dbaas.Client)(nil)
var _ dbBackupAPI = (*fakeBackupAPI)(nil)
