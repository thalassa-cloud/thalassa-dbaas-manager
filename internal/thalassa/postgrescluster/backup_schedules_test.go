package postgrescluster

import (
	"testing"

	"github.com/thalassa-cloud/client-go/dbaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func TestValidateBackupScheduleTemplates(t *testing.T) {
	ptr := func(s string) *string { return &s }
	tests := []struct {
		name    string
		in      []dbaasv1.PostgresClusterBackupScheduleTemplateSpec
		wantErr bool
	}{
		{name: "empty", in: nil, wantErr: false},
		{name: "ok", in: []dbaasv1.PostgresClusterBackupScheduleTemplateSpec{{Name: "a", Schedule: "0 * * * *"}}, wantErr: false},
		{name: "dup name", in: []dbaasv1.PostgresClusterBackupScheduleTemplateSpec{
			{Name: "x", Schedule: "0 * * * *"},
			{Name: "x", Schedule: "1 * * * *"},
		}, wantErr: true},
		{name: "empty name", in: []dbaasv1.PostgresClusterBackupScheduleTemplateSpec{{Name: "", Schedule: "0 * * * *"}}, wantErr: true},
		{name: "empty schedule", in: []dbaasv1.PostgresClusterBackupScheduleTemplateSpec{{Name: "n", Schedule: ""}}, wantErr: true},
		{name: "with description", in: []dbaasv1.PostgresClusterBackupScheduleTemplateSpec{
			{Name: "n", Schedule: "0 * * * *", Description: ptr("d")},
		}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackupScheduleTemplates(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBackupScheduleTemplateDrift(t *testing.T) {
	ptr := func(s string) *string { return &s }
	base := dbaasv1.PostgresClusterBackupScheduleTemplateSpec{Name: "n", Schedule: "0 2 * * *", Description: ptr("same")}
	fetched := &dbaas.DbClusterBackupSchedule{Name: "n", Schedule: "0 2 * * *", Description: ptr("same")}
	if backupScheduleTemplateDrift(base, fetched) {
		t.Fatal("expected no drift")
	}
	if !backupScheduleTemplateDrift(dbaasv1.PostgresClusterBackupScheduleTemplateSpec{Name: "n", Schedule: "1 2 * * *"}, fetched) {
		t.Fatal("expected drift on schedule")
	}
	if !backupScheduleTemplateDrift(dbaasv1.PostgresClusterBackupScheduleTemplateSpec{Name: "n", Schedule: "0 2 * * *", Description: ptr("y")}, fetched) {
		t.Fatal("expected drift on description")
	}
	if !backupScheduleTemplateDrift(base, nil) {
		t.Fatal("nil fetched must drift")
	}
}

func TestPostgresClusterManagedBackupSchedulesByName(t *testing.T) {
	got := postgresClusterManagedBackupSchedulesByName([]dbaasv1.PostgresClusterManagedBackupSchedule{
		{Name: "a", Identity: "id-a"},
		{Name: "", Identity: "skip"},
		{Name: "b", Identity: ""},
		{Name: "c", Identity: "id-c"},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got["a"] != "id-a" || got["c"] != "id-c" {
		t.Fatalf("got %#v", got)
	}
	if len(postgresClusterManagedBackupSchedulesByName(nil)) != 0 {
		t.Fatal("nil input should yield empty map")
	}
}
