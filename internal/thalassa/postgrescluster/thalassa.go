package postgrescluster

import (
	"k8s.io/apimachinery/pkg/api/equality"

	"github.com/thalassa-cloud/client-go/dbaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	helpers "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/helpers"
)

func (h *Handler) specToPostgresInitDb(in *dbaasv1.PostgresInitDbSpec) *dbaas.PostgresInitDb {
	if in == nil {
		return nil
	}
	return &dbaas.PostgresInitDb{
		DataChecksums: in.DataChecksums,
		Encoding:      in.Encoding,
		Locale:        in.Locale,
		LcCollate:     in.LocaleCollate,
		LcCtype:       in.LocaleCType,
	}
}

func (h *Handler) specToCreateRequest(pg *dbaasv1.PostgresCluster, subnetIdentity string, sgIdentities []string, engineVersion string, objectStoreID string) dbaas.CreateDbClusterRequest {
	req := dbaas.CreateDbClusterRequest{
		Name:                         helpers.EffectiveName(pg.Name, pg.Spec.Metadata),
		Description:                  pg.Spec.Description,
		Labels:                       helpers.EffectiveLabelsDbaas(pg.Spec.Metadata),
		SubnetIdentity:               subnetIdentity,
		SecurityGroupAttachments:     sgIdentities,
		DeleteProtection:             pg.Spec.DeleteProtection,
		Engine:                       dbaas.DbClusterDatabaseEnginePostgres,
		EngineVersion:                engineVersion,
		Parameters:                   pg.Spec.Parameters,
		AllocatedStorage:             uint64(pg.Spec.StorageGB),
		VolumeTypeClassIdentity:      pg.Spec.VolumeTypeClassId,
		DatabaseInstanceTypeIdentity: pg.Spec.InstanceType.ID,
		Replicas:                     int(pg.Spec.Instances),
	}
	if objectStoreID != "" {
		id := objectStoreID
		req.DbObjectStoreIdentity = &id
	}
	if pg.Spec.MaintenanceDay != nil {
		d := uint(*pg.Spec.MaintenanceDay)
		req.MaintenanceDay = &d
	}
	if pg.Spec.MaintenanceStartAt != nil {
		s := uint(*pg.Spec.MaintenanceStartAt)
		req.MaintenanceStartAt = &s
	}
	req.PostgresInitDb = h.specToPostgresInitDb(pg.Spec.InitDb)
	return req
}

func (h *Handler) specToUpdateRequest(pg *dbaasv1.PostgresCluster, sgIdentities []string, objectStoreID string) dbaas.UpdateDbClusterRequest {
	req := dbaas.UpdateDbClusterRequest{
		Name:                     helpers.EffectiveName(pg.Name, pg.Spec.Metadata),
		Description:              pg.Spec.Description,
		Labels:                   dbaas.Labels(helpers.EffectiveLabelsDbaas(pg.Spec.Metadata)),
		Annotations:              dbaas.Annotations(helpers.EffectiveAnnotationsDbaas(pg.Spec.Metadata)),
		SecurityGroupAttachments: sgIdentities,
		DeleteProtection:         pg.Spec.DeleteProtection,
		Parameters:               pg.Spec.Parameters,
		AllocatedStorage:         uint64(pg.Spec.StorageGB),
		Replicas:                 int(pg.Spec.Instances),
	}
	if objectStoreID != "" {
		id := objectStoreID
		req.DbObjectStoreIdentity = &id
	}
	if pg.Spec.MaintenanceDay != nil {
		d := uint(*pg.Spec.MaintenanceDay)
		req.MaintenanceDay = &d
	}
	if pg.Spec.MaintenanceStartAt != nil {
		s := uint(*pg.Spec.MaintenanceStartAt)
		req.MaintenanceStartAt = &s
	}
	return req
}

func dbClusterObjectStoreIdentity(c *dbaas.DbCluster) string {
	if c == nil || c.DbObjectStore == nil {
		return ""
	}
	return c.DbObjectStore.Identity
}

func dbObjectStoreSpecDrift(desiredIdentity, fetchedIdentity string) bool {
	if desiredIdentity == fetchedIdentity {
		return false
	}
	if desiredIdentity == "" && fetchedIdentity != "" {
		return false
	}
	return true
}

func (h *Handler) requiresUpdate(pg *dbaasv1.PostgresCluster, fetched *dbaas.DbCluster, sgIdentities []string, objectStoreID string) bool {
	if helpers.EffectiveName(pg.Name, pg.Spec.Metadata) != fetched.Name {
		return true
	}
	if pg.Spec.Description != fetched.Description {
		return true
	}
	if pg.Spec.DeleteProtection != fetched.DeleteProtection {
		return true
	}
	if int(pg.Spec.Instances) != fetched.Replicas {
		return true
	}
	if uint64(pg.Spec.StorageGB) != fetched.AllocatedStorage {
		return true
	}
	if dbObjectStoreSpecDrift(objectStoreID, dbClusterObjectStoreIdentity(fetched)) {
		return true
	}
	fetchedIDs := make([]string, 0, len(fetched.SecurityGroups))
	for _, sg := range fetched.SecurityGroups {
		fetchedIDs = append(fetchedIDs, sg.Identity)
	}
	return !equality.Semantic.DeepEqual(sgIdentities, fetchedIDs)
}
