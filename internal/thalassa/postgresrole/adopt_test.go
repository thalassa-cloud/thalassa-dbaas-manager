package postgresrole

import (
	"errors"
	"testing"

	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"
)

func TestIsPgRoleAlreadyExists(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "bad request already exists",
			err:  errors.Join(thalassaclient.ErrBadRequest, errors.New("role already exists")),
			want: true,
		},
		{
			name: "bad request other",
			err:  errors.Join(thalassaclient.ErrBadRequest, errors.New("invalid name")),
			want: false,
		},
		{
			name: "not found",
			err:  thalassaclient.ErrNotFound,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPgRoleAlreadyExists(tt.err); got != tt.want {
				t.Fatalf("isPgRoleAlreadyExists() = %v, want %v", got, tt.want)
			}
		})
	}
}
