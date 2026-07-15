package postgresrole

import (
	"fmt"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-dbaas-manager/internal/secretref"
)

// ValidateSecretRefs checks PasswordSecretRef and WriteConnectionSecretToRef namespaces.
// When allowAllNamespaces is false, refs must be empty or equal to the role namespace.
func ValidateSecretRefs(role *dbaasv1.PostgresRole, allowAllNamespaces bool) error {
	if role == nil {
		return fmt.Errorf("role is nil")
	}
	if role.Spec.PasswordSecretRef != nil {
		if _, err := secretref.Resolve(role.Namespace, role.Spec.PasswordSecretRef.Namespace, allowAllNamespaces); err != nil {
			return fmt.Errorf("passwordSecretRef: %w", err)
		}
	}
	if role.Spec.WriteConnectionSecretToRef != nil {
		if _, err := secretref.Resolve(role.Namespace, role.Spec.WriteConnectionSecretToRef.Namespace, allowAllNamespaces); err != nil {
			return fmt.Errorf("writeConnectionSecretToRef: %w", err)
		}
	}
	return nil
}
