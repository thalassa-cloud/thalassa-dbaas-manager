package postgresrole

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

const (
	connectionSecretKeyUsername = "username"
	connectionSecretKeyPassword = "password"
	connectionSecretKeyHost     = "host"
	connectionSecretKeyPort     = "port"
	connectionSecretKeyURI      = "uri"
	generatedPasswordLength     = 32
)

// ResolvePassword returns the password to use for the role: generated if GeneratePassword and WriteConnectionSecretToRef are set,
// otherwise from PasswordSecretRef if set, otherwise empty.
func ResolvePassword(ctx context.Context, c client.Client, role *dbaasv1.PostgresRole) (string, error) {
	if role.Spec.WriteConnectionSecretToRef != nil && role.Spec.GeneratePassword {
		return generateRandomPassword(generatedPasswordLength)
	}
	if role.Spec.PasswordSecretRef == nil {
		return "", nil
	}
	ns := role.Spec.PasswordSecretRef.Namespace
	if ns == "" {
		ns = role.Namespace
	}
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: role.Spec.PasswordSecretRef.Name}, &secret); err != nil {
		return "", fmt.Errorf("get password secret %s/%s: %w", ns, role.Spec.PasswordSecretRef.Name, err)
	}
	b, ok := secret.Data[role.Spec.PasswordSecretRef.Key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s has no key %q", ns, role.Spec.PasswordSecretRef.Name, role.Spec.PasswordSecretRef.Key)
	}
	return string(b), nil
}

func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// reconcileConnectionSecret creates or updates the Secret referenced by WriteConnectionSecretToRef with connection details.
// Cluster must be referenced by name (not only identity) so the controller can read endpoint from PostgresCluster status.
func (h *Handler) reconcileConnectionSecret(ctx context.Context, role *dbaasv1.PostgresRole, clusterIdentity, password string) error {
	if role.Spec.WriteConnectionSecretToRef == nil {
		return nil
	}
	if role.Spec.ClusterRef.Name == "" {
		return fmt.Errorf("writeConnectionSecretToRef requires clusterRef.name to resolve cluster endpoint")
	}
	ref := role.Spec.WriteConnectionSecretToRef
	ns := ref.Namespace
	if ns == "" {
		ns = role.Namespace
	}
	var cluster dbaasv1.PostgresCluster
	clusterNS := role.Spec.ClusterRef.Namespace
	if clusterNS == "" {
		clusterNS = role.Namespace
	}
	if err := h.Client.Get(ctx, types.NamespacedName{Namespace: clusterNS, Name: role.Spec.ClusterRef.Name}, &cluster); err != nil {
		return fmt.Errorf("get PostgresCluster for endpoint: %w", err)
	}
	if cluster.Status.ResourceID != clusterIdentity {
		return nil
	}
	host := cluster.Status.EndpointHost
	port := cluster.Status.Port
	if port == 0 {
		port = 5432
	}
	uri := ""
	if host != "" {
		u := &url.URL{
			Scheme: "postgresql",
			Host:   fmt.Sprintf("%s:%d", host, port),
			Path:   "/",
		}
		if password != "" {
			u.User = url.UserPassword(role.Spec.Name, password)
		} else {
			u.User = url.User(role.Spec.Name)
		}
		uri = u.String()
	}
	data := map[string][]byte{
		connectionSecretKeyUsername: []byte(role.Spec.Name),
		connectionSecretKeyHost:     []byte(host),
		connectionSecretKeyPort:     []byte(fmt.Sprintf("%d", port)),
	}
	if password != "" {
		data[connectionSecretKeyPassword] = []byte(password)
	}
	if uri != "" {
		data[connectionSecretKeyURI] = []byte(uri)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: ref.Name},
		Data:       data,
	}
	_, createErr := controllerutil.CreateOrUpdate(ctx, h.Client, secret, func() error {
		secret.Data = data
		if role.Namespace == secret.Namespace {
			return controllerutil.SetControllerReference(role, secret, h.Scheme)
		}
		return nil
	})
	return createErr
}
