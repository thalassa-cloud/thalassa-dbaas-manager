package postgrescluster

import (
	"context"
	"fmt"
	"strings"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) resolveVolumeTypeClassID(ctx context.Context, volumeTypeClassID string) (string, error) {
	volumeTypes, err := h.IaaSClient.ListVolumeTypes(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("list volume types: %w", err)
	}
	for _, v := range volumeTypes {
		if v.Identity == volumeTypeClassID || strings.EqualFold(v.Name, volumeTypeClassID) {
			return v.Identity, nil
		}
	}
	return "", fmt.Errorf("volume type class %q not found", volumeTypeClassID)
}

func (h *Handler) setPostgresClusterAnnotation(ctx context.Context, pg *dbaasv1.PostgresCluster, key, value string) error {
	var latest dbaasv1.PostgresCluster
	if err := h.Client.Get(ctx, client.ObjectKeyFromObject(pg), &latest); err != nil {
		return err
	}
	base := latest.DeepCopy()
	if latest.Annotations == nil {
		latest.Annotations = map[string]string{}
	}
	latest.Annotations[key] = value
	return h.Client.Patch(ctx, &latest, client.MergeFrom(base))
}

func (h *Handler) removePostgresClusterAnnotation(ctx context.Context, pg *dbaasv1.PostgresCluster) error {
	key := PreDeleteBackupIdentityAnnotation
	var latest dbaasv1.PostgresCluster
	if err := h.Client.Get(ctx, client.ObjectKeyFromObject(pg), &latest); err != nil {
		return err
	}
	if latest.Annotations == nil || latest.Annotations[key] == "" {
		return nil
	}
	base := latest.DeepCopy()
	delete(latest.Annotations, key)
	if len(latest.Annotations) == 0 {
		latest.Annotations = nil
	}
	return h.Client.Patch(ctx, &latest, client.MergeFrom(base))
}
