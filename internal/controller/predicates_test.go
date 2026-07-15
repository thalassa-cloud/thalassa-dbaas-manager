package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	thalassahelpers "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/helpers"
)

func TestPrimaryResourcePredicate_Update(t *testing.T) {
	t.Parallel()

	pred := PrimaryResourcePredicate()
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name string
		old  *dbaasv1.PostgresCluster
		new  *dbaasv1.PostgresCluster
		want bool
	}{
		{
			name: "status-only update ignored",
			old: &dbaasv1.PostgresCluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     dbaasv1.PostgresClusterStatus{EndpointHost: ""},
			},
			new: &dbaasv1.PostgresCluster{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     dbaasv1.PostgresClusterStatus{EndpointHost: "10.0.0.1"},
			},
			want: false,
		},
		{
			name: "spec generation change triggers",
			old:  &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new:  &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 2}},
			want: true,
		},
		{
			name: "deletion timestamp triggers",
			old:  &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{
				Generation:        1,
				DeletionTimestamp: &now,
			}},
			want: true,
		},
		{
			name: "suspend annotation added triggers",
			old:  &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
				Annotations: map[string]string{
					thalassahelpers.SuspendAnnotationKey: "true",
				},
			}},
			want: true,
		},
		{
			name: "resume (suspend removed) triggers",
			old: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
				Annotations: map[string]string{
					thalassahelpers.SuspendAnnotationKey: "true",
				},
			}},
			new:  &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			want: true,
		},
		{
			name: "unrelated annotation ignored",
			old:  &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
			new: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
				Annotations: map[string]string{
					"example.com/note": "changed",
				},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pred.Update(event.UpdateEvent{ObjectOld: tt.old, ObjectNew: tt.new})
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOwnedResourcePredicate_Delete(t *testing.T) {
	t.Parallel()

	pred := OwnedResourcePredicate()
	assert.True(t, pred.Delete(event.DeleteEvent{
		Object: &dbaasv1.PostgresCluster{},
	}))
	assert.False(t, PrimaryResourcePredicate().Delete(event.DeleteEvent{
		Object: &dbaasv1.PostgresCluster{},
	}))
}

func TestOwnedResourcePredicate_Update(t *testing.T) {
	t.Parallel()

	pred := OwnedResourcePredicate()
	assert.False(t, pred.Update(event.UpdateEvent{
		ObjectOld: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
		ObjectNew: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
	}))
	assert.True(t, pred.Update(event.UpdateEvent{
		ObjectOld: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 1}},
		ObjectNew: &dbaasv1.PostgresCluster{ObjectMeta: metav1.ObjectMeta{Generation: 2}},
	}))
}
