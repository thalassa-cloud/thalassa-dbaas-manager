package postgresrole

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-dbaas-manager/internal/secretref"
)

func TestValidateSecretRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		role               *dbaasv1.PostgresRole
		allowAllNamespaces bool
		wantErr            error
	}{
		{
			name: "nil refs ok",
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app"},
			},
		},
		{
			name: "same namespace password secret ok",
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app"},
				Spec: dbaasv1.PostgresRoleSpec{
					PasswordSecretRef: &dbaasv1.SecretKeySelector{
						Name:      "pw",
						Key:       "password",
						Namespace: "app",
					},
				},
			},
		},
		{
			name: "cross namespace password secret denied",
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app"},
				Spec: dbaasv1.PostgresRoleSpec{
					PasswordSecretRef: &dbaasv1.SecretKeySelector{
						Name:      "pw",
						Key:       "password",
						Namespace: "kube-system",
					},
				},
			},
			wantErr: secretref.ErrCrossNamespaceForbidden,
		},
		{
			name: "cross namespace write secret denied",
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app"},
				Spec: dbaasv1.PostgresRoleSpec{
					WriteConnectionSecretToRef: &dbaasv1.SecretReference{
						Name:      "conn",
						Namespace: "other",
					},
				},
			},
			wantErr: secretref.ErrCrossNamespaceForbidden,
		},
		{
			name: "cross namespace allowed when enabled",
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app"},
				Spec: dbaasv1.PostgresRoleSpec{
					PasswordSecretRef: &dbaasv1.SecretKeySelector{
						Name:      "pw",
						Key:       "password",
						Namespace: "other",
					},
					WriteConnectionSecretToRef: &dbaasv1.SecretReference{
						Name:      "conn",
						Namespace: "other",
					},
				},
			},
			allowAllNamespaces: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSecretRefs(tt.role, tt.allowAllNamespaces)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
		})
	}
}
