package secretref

import (
	"errors"
	"fmt"
)

// ErrCrossNamespaceForbidden is returned when a secret reference targets another
// namespace while cross-namespace secret refs are disabled on the manager.
var ErrCrossNamespaceForbidden = errors.New("cross-namespace secret reference is not allowed")

// Resolve returns the namespace to use for a secret reference.
// An empty refNamespace defaults to resourceNamespace.
// When allowAllNamespaces is false, a non-empty refNamespace that differs from
// resourceNamespace is rejected (confused-deputy prevention).
func Resolve(resourceNamespace, refNamespace string, allowAllNamespaces bool) (string, error) {
	if refNamespace == "" || refNamespace == resourceNamespace {
		return resourceNamespace, nil
	}
	if !allowAllNamespaces {
		return "", fmt.Errorf("%w: referenced namespace %q differs from resource namespace %q (enable --allow-all-namespaces-secret-ref to permit)",
			ErrCrossNamespaceForbidden, refNamespace, resourceNamespace)
	}
	return refNamespace, nil
}
