package postgresrole

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// ErrEndpointNotReady is returned when WriteConnectionSecretToRef is set but the referenced PostgresCluster has no endpoint host yet.
var ErrEndpointNotReady = errors.New("PostgresCluster endpoint is not available yet")

// ResolvePassword returns the password to use for the role: generated once when password
// generation is enabled, otherwise from PasswordSecretRef if set, otherwise empty.
func ResolvePassword(ctx context.Context, c client.Client, role *dbaasv1.PostgresRole) (string, error) {
	if shouldGeneratePassword(role) {
		stored, err := passwordFromConnectionSecret(ctx, c, role)
		if err != nil {
			return "", err
		}
		if stored != "" {
			return stored, nil
		}
		if role.Status.ResourceID != "" {
			// Role already exists but the password was not persisted (e.g. prior reconcile bug).
			// Rotate to a new password so the connection secret can be repaired.
			return generateRandomPassword(generatedPasswordLength)
		}
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

// shouldGeneratePassword is true when the controller should create a password and store it in
// writeConnectionSecretToRef. PasswordSecretRef always takes precedence over generation.
func shouldGeneratePassword(role *dbaasv1.PostgresRole) bool {
	return role.Spec.WriteConnectionSecretToRef != nil && role.Spec.PasswordSecretRef == nil
}

func connectionSecretNamespacedName(role *dbaasv1.PostgresRole) (types.NamespacedName, bool) {
	if role.Spec.WriteConnectionSecretToRef == nil {
		return types.NamespacedName{}, false
	}
	ref := role.Spec.WriteConnectionSecretToRef
	ns := ref.Namespace
	if ns == "" {
		ns = role.Namespace
	}
	return types.NamespacedName{Namespace: ns, Name: ref.Name}, true
}

func passwordFromConnectionSecret(ctx context.Context, c client.Client, role *dbaasv1.PostgresRole) (string, error) {
	key, ok := connectionSecretNamespacedName(role)
	if !ok {
		return "", nil
	}
	var secret corev1.Secret
	if err := c.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("get connection secret %s/%s: %w", key.Namespace, key.Name, err)
	}
	b, ok := secret.Data[connectionSecretKeyPassword]
	if !ok {
		return "", nil
	}
	return string(b), nil
}

func (h *Handler) WriteConnectionSecretCredentials(ctx context.Context, role *dbaasv1.PostgresRole, password string) error {
	return h.writeConnectionSecretCredentials(ctx, role, password)
}

func (h *Handler) writeConnectionSecretCredentials(ctx context.Context, role *dbaasv1.PostgresRole, password string) error {
	key, ok := connectionSecretNamespacedName(role)
	if !ok || password == "" {
		return nil
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, h.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[connectionSecretKeyUsername] = []byte(role.Spec.Name)
		secret.Data[connectionSecretKeyPassword] = []byte(password)
		if role.Namespace == secret.Namespace {
			return controllerutil.SetControllerReference(role, secret, h.Scheme)
		}
		return nil
	})
	return err
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
	effectivePassword := password
	if effectivePassword == "" {
		var err error
		effectivePassword, err = passwordFromConnectionSecret(ctx, h.Client, role)
		if err != nil {
			return err
		}
	}
	if host == "" {
		if err := h.writeConnectionSecretCredentials(ctx, role, effectivePassword); err != nil {
			return err
		}
		return ErrEndpointNotReady
	}
	port := cluster.Status.Port
	if port == 0 {
		port = 5432
	}
	u := &url.URL{
		Scheme: "postgresql",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/",
	}
	if effectivePassword != "" {
		u.User = url.UserPassword(role.Spec.Name, effectivePassword)
	} else {
		u.User = url.User(role.Spec.Name)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: ref.Name},
	}
	_, createErr := controllerutil.CreateOrUpdate(ctx, h.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		existingPassword := secret.Data[connectionSecretKeyPassword]
		secret.Data[connectionSecretKeyUsername] = []byte(role.Spec.Name)
		secret.Data[connectionSecretKeyHost] = []byte(host)
		secret.Data[connectionSecretKeyPort] = []byte(fmt.Sprintf("%d", port))
		if len(effectivePassword) > 0 {
			secret.Data[connectionSecretKeyPassword] = []byte(effectivePassword)
		} else if len(existingPassword) > 0 {
			effectivePassword = string(existingPassword)
			u.User = url.UserPassword(role.Spec.Name, effectivePassword)
		}
		secret.Data[connectionSecretKeyURI] = []byte(u.String())
		if role.Namespace == secret.Namespace {
			return controllerutil.SetControllerReference(role, secret, h.Scheme)
		}
		return nil
	})
	return createErr
}
