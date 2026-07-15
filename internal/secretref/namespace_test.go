package secretref

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		resourceNamespace  string
		refNamespace       string
		allowAllNamespaces bool
		want               string
		wantErr            error
	}{
		{
			name:              "empty ref defaults to resource namespace",
			resourceNamespace: "app",
			refNamespace:      "",
			want:              "app",
		},
		{
			name:              "same namespace allowed when restricted",
			resourceNamespace: "app",
			refNamespace:      "app",
			want:              "app",
		},
		{
			name:               "cross namespace denied by default",
			resourceNamespace:  "app",
			refNamespace:       "other",
			allowAllNamespaces: false,
			wantErr:            ErrCrossNamespaceForbidden,
		},
		{
			name:               "cross namespace allowed when enabled",
			resourceNamespace:  "app",
			refNamespace:       "other",
			allowAllNamespaces: true,
			want:               "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Resolve(tt.resourceNamespace, tt.refNamespace, tt.allowAllNamespaces)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolve_errorMessageMentionsFlag(t *testing.T) {
	t.Parallel()

	_, err := Resolve("app", "other", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCrossNamespaceForbidden))
	assert.Contains(t, err.Error(), "--allow-all-namespaces-secret-ref")
}
