package postgrescluster

import (
	"context"
	"fmt"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

// embeddedBackupScheduleMethod is the backup method for schedules created from PostgresCluster.spec.backupSchedules.
const embeddedBackupScheduleMethod = dbaas.DbClusterBackupScheduleMethodBarman

func validateBackupScheduleTemplates(templates []dbaasv1.PostgresClusterBackupScheduleTemplateSpec) error {
	seen := make(map[string]struct{}, len(templates))
	for i := range templates {
		n := templates[i].Name
		if n == "" {
			return fmt.Errorf("backupSchedules[%d]: name is required", i)
		}
		if _, ok := seen[n]; ok {
			return fmt.Errorf("duplicate backup schedule name %q in spec.backupSchedules", n)
		}
		seen[n] = struct{}{}
		if templates[i].Schedule == "" {
			return fmt.Errorf("backupSchedules[%q]: schedule is required", n)
		}
	}
	return nil
}

func backupScheduleTemplateToCreateRequest(t dbaasv1.PostgresClusterBackupScheduleTemplateSpec) dbaas.CreateDbBackupScheduleRequest {
	return dbaas.CreateDbBackupScheduleRequest{
		Name:            t.Name,
		Description:     t.Description,
		Schedule:        t.Schedule,
		RetentionPolicy: "7d", // ignored, but required by api
		Method:          embeddedBackupScheduleMethod,
	}
}

func backupScheduleTemplateToUpdateRequest(t dbaasv1.PostgresClusterBackupScheduleTemplateSpec, preserveRetention string) dbaas.UpdateDbBackupScheduleRequest {
	desc := ""
	if t.Description != nil {
		desc = *t.Description
	}
	return dbaas.UpdateDbBackupScheduleRequest{
		Name:            t.Name,
		Description:     desc,
		Schedule:        t.Schedule,
		RetentionPolicy: preserveRetention,
	}
}

func backupScheduleTemplateDrift(t dbaasv1.PostgresClusterBackupScheduleTemplateSpec, fetched *dbaas.DbClusterBackupSchedule) bool {
	if fetched == nil {
		return true
	}
	if t.Name != fetched.Name {
		return true
	}
	if t.Schedule != fetched.Schedule {
		return true
	}
	wantDesc := ""
	if t.Description != nil {
		wantDesc = *t.Description
	}
	gotDesc := ""
	if fetched.Description != nil {
		gotDesc = *fetched.Description
	}
	return wantDesc != gotDesc
}

// reconcileEmbeddedBackupSchedules ensures Thalassa backup schedules for spec.backupSchedules exist and match the template (name, schedule, description).
// On update, the existing retention policy in Thalassa is preserved.
// Schedules created here are recorded in status.managedBackupSchedules; only those are deleted when removed from spec.
func (h *Handler) reconcileEmbeddedBackupSchedules(ctx context.Context, clusterIdentity string, pg *dbaasv1.PostgresCluster) error {
	templates := pg.Spec.BackupSchedules
	oldManaged := postgresClusterManagedBackupSchedulesByName(pg.Status.ManagedBackupSchedules)

	if len(templates) == 0 {
		for name, id := range oldManaged {
			if err := h.DbaasClient.DeleteDbBackupSchedule(ctx, clusterIdentity, id); err != nil && !thalassaclient.IsNotFound(err) {
				return fmt.Errorf("delete backup schedule %q (%s): %w", name, id, err)
			}
		}
		pg.Status.ManagedBackupSchedules = nil
		return nil
	}

	if err := validateBackupScheduleTemplates(templates); err != nil {
		return err
	}

	desiredByName := make(map[string]struct{}, len(templates))
	for i := range templates {
		desiredByName[templates[i].Name] = struct{}{}
	}

	list, err := h.DbaasClient.ListDbBackupSchedules(ctx, clusterIdentity, nil)
	if err != nil {
		return fmt.Errorf("list backup schedules: %w", err)
	}
	byName := make(map[string]*dbaas.DbClusterBackupSchedule, len(list))
	for i := range list {
		s := &list[i]
		byName[s.Name] = s
	}

	for name, id := range oldManaged {
		if _, stillDesired := desiredByName[name]; stillDesired {
			continue
		}
		if err := h.DbaasClient.DeleteDbBackupSchedule(ctx, clusterIdentity, id); err != nil && !thalassaclient.IsNotFound(err) {
			return fmt.Errorf("delete backup schedule %q (%s): %w", name, id, err)
		}
		delete(byName, name)
	}

	newManaged := make([]dbaasv1.PostgresClusterManagedBackupSchedule, 0, len(templates))
	for i := range templates {
		t := &templates[i]
		existing := byName[t.Name]
		if existing == nil {
			req := backupScheduleTemplateToCreateRequest(*t)
			created, err := h.DbaasClient.CreateDbBackupSchedule(ctx, clusterIdentity, req)
			if err != nil {
				return fmt.Errorf("create backup schedule %q: %w", t.Name, err)
			}
			byName[t.Name] = created
			newManaged = append(newManaged, dbaasv1.PostgresClusterManagedBackupSchedule{Name: t.Name, Identity: created.Identity})
			continue
		}
		if backupScheduleTemplateDrift(*t, existing) {
			_, err := h.DbaasClient.UpdateDbBackupSchedule(ctx, clusterIdentity, existing.Identity, backupScheduleTemplateToUpdateRequest(*t, existing.RetentionPolicy))
			if err != nil {
				return fmt.Errorf("update backup schedule %q: %w", t.Name, err)
			}
		}
		if _, wasManaged := oldManaged[t.Name]; wasManaged {
			newManaged = append(newManaged, dbaasv1.PostgresClusterManagedBackupSchedule{Name: t.Name, Identity: existing.Identity})
		}
	}

	pg.Status.ManagedBackupSchedules = newManaged
	return nil
}

func postgresClusterManagedBackupSchedulesByName(entries []dbaasv1.PostgresClusterManagedBackupSchedule) map[string]string {
	out := make(map[string]string, len(entries))
	for i := range entries {
		e := &entries[i]
		if e.Name == "" || e.Identity == "" {
			continue
		}
		out[e.Name] = e.Identity
	}
	return out
}
