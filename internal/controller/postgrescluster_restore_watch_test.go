package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func TestEnqueueRestoresForSourceCluster(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	require.NoError(t, dbaasv1.AddToScheme(scheme))

	source := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "db"},
		Status:     dbaasv1.PostgresClusterStatus{ResourceID: "db-source"},
	}
	byName := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "copy", Namespace: "db"},
		Spec: dbaasv1.PostgresClusterSpec{
			Restore: &dbaasv1.PostgresClusterRestoreSpec{
				SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "source"},
			},
		},
	}
	byIdentity := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "apps"},
		Spec: dbaasv1.PostgresClusterSpec{
			Restore: &dbaasv1.PostgresClusterRestoreSpec{
				SourceClusterRef: &dbaasv1.PostgresClusterRef{Identity: "db-source"},
			},
		},
	}
	unrelated := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "db"},
	}
	withBackupOnly := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "from-backup", Namespace: "db"},
		Spec: dbaasv1.PostgresClusterSpec{
			Restore: &dbaasv1.PostgresClusterRestoreSpec{BackupIdentity: "backup-given"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source, byName, byIdentity, unrelated, withBackupOnly).Build()
	r := &PostgresClusterReconciler{Client: c}

	got := r.enqueueRestoresForSourceCluster(context.Background(), source)
	assert.ElementsMatch(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Namespace: "db", Name: "copy"}},
		{NamespacedName: types.NamespacedName{Namespace: "apps", Name: "other"}},
	}, got)

	source.Status.ResourceID = ""
	assert.Empty(t, r.enqueueRestoresForSourceCluster(context.Background(), source))
}

func TestRestoreSourcePredicate(t *testing.T) {
	t.Parallel()

	pred := restoreSourcePredicate()
	withID := &dbaasv1.PostgresCluster{Status: dbaasv1.PostgresClusterStatus{ResourceID: "db-1"}}
	withoutID := &dbaasv1.PostgresCluster{}

	assert.True(t, pred.Create(event.CreateEvent{Object: withID}))
	assert.False(t, pred.Create(event.CreateEvent{Object: withoutID}))
	assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: withoutID, ObjectNew: withID}))
	assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: withID, ObjectNew: withID}))
	assert.False(t, pred.Delete(event.DeleteEvent{Object: withID}))
}

func TestRestoreReferencesSourceNamespace(t *testing.T) {
	t.Parallel()

	source := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "db"},
		Status:     dbaasv1.PostgresClusterStatus{ResourceID: "db-source"},
	}
	tests := []struct {
		name string
		item *dbaasv1.PostgresCluster
		want bool
	}{
		{
			name: "same namespace name",
			item: &dbaasv1.PostgresCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "copy", Namespace: "db"},
				Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
					SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "source"},
				}},
			},
			want: true,
		},
		{
			name: "explicit other namespace misses",
			item: &dbaasv1.PostgresCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "copy", Namespace: "apps"},
				Spec: dbaasv1.PostgresClusterSpec{Restore: &dbaasv1.PostgresClusterRestoreSpec{
					SourceClusterRef: &dbaasv1.PostgresClusterRef{Name: "source", Namespace: "other"},
				}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, restoreReferencesSource(tt.item, source))
		})
	}
}
