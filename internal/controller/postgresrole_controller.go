/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/thalassa-cloud/client-go/dbaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	pgref "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresclusterref"
	pgrole "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresrole"
)

// PostgresRoleReconciler wires Kubernetes reconciliation to the Thalassa postgresrole.Handler.
type PostgresRoleReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	DbaasClient                 *dbaas.Client
	Recorder                    record.EventRecorder
	Handler                     *pgrole.Handler
	AllowAllNamespacesSecretRef bool
}

// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresroles/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

// Reconcile reconciles a PostgresRole with the Thalassa DB cluster.
func (r *PostgresRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var role dbaasv1.PostgresRole
	if err := r.Get(ctx, req.NamespacedName, &role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.DbaasClient == nil {
		log.Info("DBaaS client not configured, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if IsSuspended(&role) {
		return ctrl.Result{}, nil
	}

	role.Status.ObservedGeneration, role.Status.LastReconcileTime = ReconcileMeta(role.Generation)

	if !role.DeletionTimestamp.IsZero() {
		return r.Handler.Terminate(ctx, &role)
	}
	if controllerutil.AddFinalizer(&role, pgrole.Finalizer) {
		if err := r.Update(ctx, &role); err != nil {
			return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	clusterIdentity, err := pgref.Resolve(ctx, r.Client, role.Namespace, role.Spec.ClusterRef)
	if err != nil {
		if errors.Is(err, pgref.ErrDependencyNotReady) {
			return ctrl.Result{RequeueAfter: RequeueAfterDependencyNotReady}, nil
		}
		return r.Handler.SetErrorCondition(ctx, &role, "ClusterNotFound", err.Error(), err)
	}

	if err := pgrole.ValidateSecretRefs(&role, r.AllowAllNamespacesSecretRef); err != nil {
		return r.Handler.SetErrorCondition(ctx, &role, "InvalidSecretRef", err.Error(), err)
	}

	password, err := pgrole.ResolvePassword(ctx, r.Client, &role, r.AllowAllNamespacesSecretRef)
	if err != nil {
		return r.Handler.SetErrorCondition(ctx, &role, "PasswordSecretError", err.Error(), err)
	}
	if password != "" && role.Spec.WriteConnectionSecretToRef != nil {
		if err := r.Handler.WriteConnectionSecretCredentials(ctx, &role, password); err != nil {
			return r.Handler.SetErrorCondition(ctx, &role, "ConnectionSecretError", err.Error(), err)
		}
	}

	return r.Handler.Reconcile(ctx, pgrole.ReconcileInput{
		Role:            &role,
		ClusterIdentity: clusterIdentity,
		Password:        password,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgresRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("postgresrole") //nolint:staticcheck // SA1019: handlers use record.EventRecorder; events API uses different Eventf signature
	if r.Handler == nil {
		r.Handler = pgrole.NewHandler(pgrole.Config{
			Client:                      r.Client,
			Scheme:                      r.Scheme,
			DbaasClient:                 r.DbaasClient,
			Recorder:                    r.Recorder,
			AllowAllNamespacesSecretRef: r.AllowAllNamespacesSecretRef,
		})
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.PostgresRole{}).
		Owns(&corev1.Secret{}).
		Watches(
			&dbaasv1.PostgresCluster{},
			handler.EnqueueRequestsFromMapFunc(r.enqueuePostgresRolesForCluster),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					cluster, ok := e.Object.(*dbaasv1.PostgresCluster)
					return ok && cluster.Status.EndpointHost != ""
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldCluster, okOld := e.ObjectOld.(*dbaasv1.PostgresCluster)
					newCluster, okNew := e.ObjectNew.(*dbaasv1.PostgresCluster)
					if !okOld || !okNew {
						return false
					}
					return oldCluster.Status.EndpointHost != newCluster.Status.EndpointHost ||
						oldCluster.Status.Port != newCluster.Status.Port
				},
				DeleteFunc:  func(event.DeleteEvent) bool { return false },
				GenericFunc: func(event.GenericEvent) bool { return false },
			}),
		).
		Named("postgresrole").
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemFastSlowRateLimiter[reconcile.Request](1*time.Second, 10*time.Second, 15),
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
			GenericFunc: func(e event.GenericEvent) bool { return false },
		}).
		Complete(r)
}
