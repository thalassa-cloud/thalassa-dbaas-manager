package postgresgrant

import (
	"strings"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
)

func setPostgresGrantConditionFromStatus(grant *dbaasv1.PostgresGrant, status, reason string) {
	switch {
	case strings.EqualFold(status, stdconditions.ResourceStatusReady):
		stdconditions.SetStandardConditions(&grant.Status.Conditions, stdconditions.ConditionStateAvailable, "Ready", "PostgreSQL grant is ready")
	default:
		stdconditions.SetStandardConditions(&grant.Status.Conditions, stdconditions.ConditionStateProgressing, reason, firstNonEmpty(grant.Status.LastReconcileError, "PostgreSQL grant is provisioning"))
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
