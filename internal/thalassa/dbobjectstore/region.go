package dbobjectstore

import (
	"context"
	"fmt"
	"strings"

	thalassaiaas "github.com/thalassa-cloud/client-go/iaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

// resolveObjectStoreRegion picks the region for CreateDbObjectStore: spec.region, then manager default-region, then VPC region of default-subnet-id.
func (h *Handler) resolveObjectStoreRegion(ctx context.Context, obj *dbaasv1.DbObjectStore) (string, error) {
	if s := strings.TrimSpace(obj.Spec.Region); s != "" {
		return s, nil
	}
	if s := strings.TrimSpace(h.DefaultRegion); s != "" {
		return s, nil
	}
	if h.IaaSClient == nil || strings.TrimSpace(h.DefaultSubnetID) == "" {
		return "", fmt.Errorf("spec.region is empty; set spec.region, or --default-region / REGION, or default-subnet-id for discovery via IaaS API")
	}
	sn, err := h.IaaSClient.GetSubnet(ctx, strings.TrimSpace(h.DefaultSubnetID))
	if err != nil {
		return "", fmt.Errorf("discover region from default subnet: %w", err)
	}
	if rid := regionIdentityFromSubnet(sn); rid != "" {
		return rid, nil
	}
	return "", fmt.Errorf("could not read region from default subnet %q (ensure GetSubnet returns vpc.cloudRegion); set spec.region or --default-region", h.DefaultSubnetID)
}

func regionIdentityFromSubnet(sn *thalassaiaas.Subnet) string {
	if sn == nil || sn.Vpc == nil || sn.Vpc.CloudRegion == nil {
		return ""
	}
	cr := sn.Vpc.CloudRegion
	if cr.Identity != "" {
		return cr.Identity
	}
	if cr.Slug != "" {
		return cr.Slug
	}
	return ""
}
