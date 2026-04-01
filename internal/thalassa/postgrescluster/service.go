package postgrescluster

import (
	"context"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
)

func (h *Handler) reconcileExposeService(ctx context.Context, pg *dbaasv1.PostgresCluster) error {
	expose := true
	if pg.Spec.ExposeService != nil {
		expose = *pg.Spec.ExposeService
	}
	svcName := pg.Name
	if pg.Spec.ServiceName != nil && *pg.Spec.ServiceName != "" {
		svcName = *pg.Spec.ServiceName
	}
	svcKey := types.NamespacedName{Namespace: pg.Namespace, Name: svcName}

	if !expose {
		var svc corev1.Service
		if err := h.Client.Get(ctx, svcKey, &svc); err == nil {
			if metav1.IsControlledBy(&svc, pg) {
				if err := h.Client.Delete(ctx, &svc); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
		if err := h.deleteEndpointSlicesForService(ctx, pg.Namespace, svcName, pg); err != nil {
			return err
		}
		var ep corev1.Endpoints //nolint:staticcheck // SA1019: delete legacy Endpoints left from pre-EndpointSlice headless services
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: pg.Namespace, Name: svcName}, &ep); err == nil {
			if metav1.IsControlledBy(&ep, pg) {
				_ = h.Client.Delete(ctx, &ep)
			}
		}
		return nil
	}

	host := pg.Status.EndpointHost
	if host == "" {
		return nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: pg.Namespace, Name: svcName},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{Name: "postgres-rw", Port: servicePortRW, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(servicePortRW)},
				{Name: "postgres-ro", Port: servicePortRO, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(servicePortRO)},
			},
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, h.Client, svc, func() error {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.ClusterIP = "None"
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "postgres-rw", Port: servicePortRW, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(servicePortRW)},
			{Name: "postgres-ro", Port: servicePortRO, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(servicePortRO)},
		}
		return controllerutil.SetControllerReference(pg, svc, h.Scheme)
	})
	if err != nil {
		return err
	}

	rwPort := pg.Status.Port
	if rwPort == 0 {
		rwPort = servicePortRW
	}
	addr, err := endpointAddress(host)
	if err != nil {
		return err
	}
	addressType := discoveryv1.AddressTypeIPv4
	if net.ParseIP(addr).To4() == nil {
		addressType = discoveryv1.AddressTypeIPv6
	}
	sliceName := svcName + "-postgres"
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pg.Namespace,
			Name:      sliceName,
			Labels: map[string]string{
				endpointSliceServiceNameLabel: svcName,
			},
		},
		AddressType: addressType,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{addr},
				Conditions: discoveryv1.EndpointConditions{Ready: new(true)},
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{Name: new("postgres-rw"), Port: new(rwPort), Protocol: new(corev1.ProtocolTCP)},
			{Name: new("postgres-ro"), Port: new(rwPort), Protocol: new(corev1.ProtocolTCP)},
		},
	}
	if err := controllerutil.SetControllerReference(pg, slice, h.Scheme); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, h.Client, slice, func() error {
		slice.AddressType = addressType
		slice.Endpoints = []discoveryv1.Endpoint{
			{Addresses: []string{addr}, Conditions: discoveryv1.EndpointConditions{Ready: new(true)}},
		}
		slice.Ports = []discoveryv1.EndpointPort{
			{Name: new("postgres-rw"), Port: new(rwPort), Protocol: new(corev1.ProtocolTCP)},
			{Name: new("postgres-ro"), Port: new(rwPort), Protocol: new(corev1.ProtocolTCP)},
		}
		if slice.Labels == nil {
			slice.Labels = make(map[string]string)
		}
		slice.Labels[endpointSliceServiceNameLabel] = svcName
		return controllerutil.SetControllerReference(pg, slice, h.Scheme)
	})
	if err != nil {
		return err
	}
	if err := h.deleteEndpointSlicesForServiceExcept(ctx, pg.Namespace, svcName, sliceName, pg); err != nil {
		return err
	}
	return nil
}

func (h *Handler) deleteEndpointSlicesForService(ctx context.Context, namespace, svcName string, pg *dbaasv1.PostgresCluster) error {
	return h.deleteEndpointSlicesForServiceExcept(ctx, namespace, svcName, "", pg)
}

func (h *Handler) deleteEndpointSlicesForServiceExcept(ctx context.Context, namespace, svcName, exceptName string, pg *dbaasv1.PostgresCluster) error {
	var list discoveryv1.EndpointSliceList
	if err := h.Client.List(ctx, &list, client.InNamespace(namespace), client.MatchingLabels{endpointSliceServiceNameLabel: svcName}); err != nil {
		return err
	}
	for i := range list.Items {
		slice := &list.Items[i]
		if slice.Name == exceptName {
			continue
		}
		if metav1.IsControlledBy(slice, pg) {
			if err := h.Client.Delete(ctx, slice); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func endpointAddress(host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return "", fmt.Errorf("resolve %q: %w", host, err)
	}
	return addrs[0], nil
}
