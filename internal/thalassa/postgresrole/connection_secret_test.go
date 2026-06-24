package postgresrole

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func TestReconcileConnectionSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbaasv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add dbaas scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}

	cluster := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Status: dbaasv1.PostgresClusterStatus{
			ResourceID:   "cluster-1",
			EndpointHost: "10.0.0.1",
			Port:         5432,
		},
	}
	clusterNoEndpoint := &dbaasv1.PostgresCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "pg", Namespace: "default"},
		Status: dbaasv1.PostgresClusterStatus{
			ResourceID: "cluster-1",
		},
	}

	tests := []struct {
		name            string
		objects         []client.Object
		role            *dbaasv1.PostgresRole
		clusterIdentity string
		inputPassword   string
		wantErr         error
		wantSecret      bool
		wantHost        string
		wantPassword    string
	}{
		{
			name:    "skips when writeConnectionSecretToRef is unset",
			objects: []client.Object{cluster},
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: dbaasv1.PostgresRoleSpec{
					Name: "app",
					ClusterRef: dbaasv1.PostgresClusterRef{
						Name: "pg",
					},
				},
			},
			clusterIdentity: "cluster-1",
		},
		{
			name:    "waits for endpoint host",
			objects: []client.Object{clusterNoEndpoint},
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: dbaasv1.PostgresRoleSpec{
					Name: "app",
					ClusterRef: dbaasv1.PostgresClusterRef{
						Name: "pg",
					},
					WriteConnectionSecretToRef: &dbaasv1.SecretReference{
						Name: "app-credentials",
					},
				},
			},
			clusterIdentity: "cluster-1",
			inputPassword:   "secret",
			wantErr:         ErrEndpointNotReady,
			wantSecret:      true,
			wantPassword:    "secret",
		},
		{
			name: "preserves existing password when updating host",
			objects: []client.Object{
				cluster,
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "app-credentials", Namespace: "default"},
					Data: map[string][]byte{
						connectionSecretKeyUsername: []byte("app"),
						connectionSecretKeyPassword: []byte("stored-password"),
					},
				},
			},
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: dbaasv1.PostgresRoleSpec{
					Name: "app",
					ClusterRef: dbaasv1.PostgresClusterRef{
						Name: "pg",
					},
					WriteConnectionSecretToRef: &dbaasv1.SecretReference{
						Name: "app-credentials",
					},
				},
				Status: dbaasv1.PostgresRoleStatus{ResourceID: "role-1"},
			},
			clusterIdentity: "cluster-1",
			wantSecret:      true,
			wantHost:        "10.0.0.1",
			wantPassword:    "stored-password",
		},
		{
			name:    "writes connection secret when endpoint is ready",
			objects: []client.Object{cluster},
			role: &dbaasv1.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: dbaasv1.PostgresRoleSpec{
					Name: "app",
					ClusterRef: dbaasv1.PostgresClusterRef{
						Name: "pg",
					},
					WriteConnectionSecretToRef: &dbaasv1.SecretReference{
						Name: "app-credentials",
					},
				},
			},
			clusterIdentity: "cluster-1",
			inputPassword:   "secret",
			wantSecret:      true,
			wantHost:        "10.0.0.1",
			wantPassword:    "secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build(),
				Scheme: scheme,
			}

			err := h.reconcileConnectionSecret(context.Background(), tt.role, tt.clusterIdentity, tt.inputPassword)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !tt.wantSecret {
				return
			}

			var secret corev1.Secret
			if err := h.Client.Get(context.Background(), types.NamespacedName{
				Namespace: "default",
				Name:      "app-credentials",
			}, &secret); err != nil {
				t.Fatalf("get secret: %v", err)
			}
			if tt.wantHost != "" {
				if got := string(secret.Data[connectionSecretKeyHost]); got != tt.wantHost {
					t.Fatalf("host = %q, want %q", got, tt.wantHost)
				}
			} else if _, ok := secret.Data[connectionSecretKeyHost]; ok {
				t.Fatalf("host should not be set yet")
			}
			if got := string(secret.Data[connectionSecretKeyUsername]); got != "app" {
				t.Fatalf("username = %q, want app", got)
			}
			if tt.wantPassword != "" {
				if got := string(secret.Data[connectionSecretKeyPassword]); got != tt.wantPassword {
					t.Fatalf("password = %q, want %q", got, tt.wantPassword)
				}
			}
		})
	}
}

func TestResolvePasswordPreservesGeneratedPassword(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := dbaasv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add dbaas scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "app-credentials", Namespace: "default"},
		Data: map[string][]byte{
			connectionSecretKeyPassword: []byte("stored-password"),
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSecret).Build()

	role := &dbaasv1.PostgresRole{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: dbaasv1.PostgresRoleSpec{
			Name:             "app",
			GeneratePassword: true,
			WriteConnectionSecretToRef: &dbaasv1.SecretReference{
				Name: "app-credentials",
			},
		},
		Status: dbaasv1.PostgresRoleStatus{ResourceID: "role-1"},
	}

	password, err := ResolvePassword(context.Background(), c, role)
	if err != nil {
		t.Fatalf("ResolvePassword: %v", err)
	}
	if password != "stored-password" {
		t.Fatalf("password = %q, want stored-password", password)
	}
}
