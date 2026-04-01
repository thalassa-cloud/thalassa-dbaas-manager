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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaiaas "github.com/thalassa-cloud/client-go/iaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-dbaas-manager/internal/engineversion"
	pgcluster "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgrescluster"
)

// PostgresClusterReconciler wires Kubernetes reconciliation to the Thalassa postgrescluster.Handler.
type PostgresClusterReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	DbaasClient            *dbaas.Client
	IaaSClient             *thalassaiaas.Client
	Recorder               record.EventRecorder
	ClusterID              string
	DefaultSubnetID        string
	DefaultSecurityGroupID string
	DefaultDbObjectStoreID string

	Handler pgcluster.Reconciler
}

// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=dbobjectstores,verbs=get;list;watch
// +kubebuilder:rbac:groups=iaas.controllers.thalassa.cloud,resources=subnets,verbs=get;list;watch
// +kubebuilder:rbac:groups=iaas.controllers.thalassa.cloud,resources=securitygroups,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the current state of a PostgresCluster toward the desired spec.
func (r *PostgresClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var pg dbaasv1.PostgresCluster
	if err := r.Get(ctx, req.NamespacedName, &pg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.DbaasClient == nil {
		log.Info("DBaaS client not configured, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if IsSuspended(&pg) {
		return ctrl.Result{}, nil
	}

	pg.Status.ObservedGeneration, pg.Status.LastReconcileTime = ReconcileMeta(pg.Generation)

	if !pg.DeletionTimestamp.IsZero() {
		return r.Handler.Terminate(ctx, &pg)
	}
	if controllerutil.AddFinalizer(&pg, pgcluster.Finalizer) {
		if err := r.Update(ctx, &pg); err != nil {
			return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	subnetIdentity, err := r.resolveSubnetIdentity(ctx, pg.Namespace, &pg)
	if err != nil {
		return r.setPostgresClusterErrorCondition(ctx, &pg, "SubnetNotFound", err.Error(), err)
	}
	sgIdentities, err := r.resolveSecurityGroupRefs(ctx, pg.Namespace, pg.Spec.SecurityGroupRefs)
	if err != nil {
		if errors.Is(err, ErrDependencyNotReady) {
			return ctrl.Result{RequeueAfter: RequeueAfterDependencyNotReady}, nil
		}
		return r.setPostgresClusterErrorCondition(ctx, &pg, "SecurityGroupNotFound", err.Error(), err)
	}
	objectStoreID, err := r.resolveDbObjectStoreIdentity(ctx, pg.Namespace, &pg)
	if err != nil {
		if errors.Is(err, ErrDependencyNotReady) {
			return ctrl.Result{RequeueAfter: RequeueAfterDependencyNotReady}, nil
		}
		return r.setPostgresClusterErrorCondition(ctx, &pg, "DbObjectStoreNotFound", err.Error(), err)
	}
	engineVersion, err := r.resolveEngineVersion(ctx, pg.Spec.PostgresVersion)
	if err != nil {
		return r.setPostgresClusterErrorCondition(ctx, &pg, "EngineVersionNotFound", err.Error(), err)
	}

	return r.Handler.Reconcile(ctx, pgcluster.ReconcileInput{
		PG:             &pg,
		SubnetIdentity: subnetIdentity,
		SGIdentities:   sgIdentities,
		ObjectStoreID:  objectStoreID,
		EngineVersion:  engineVersion,
	})
}

func (r *PostgresClusterReconciler) resolveEngineVersionIdentity(ctx context.Context, postgresVersion string) (string, error) {
	list, err := r.DbaasClient.ListEngineVersions(ctx, dbaas.DbClusterDatabaseEnginePostgres, nil)
	if err != nil {
		return "", fmt.Errorf("list engine versions: %w", err)
	}
	versions := make([]engineversion.Version, 0, len(list))
	for _, v := range list {
		versions = append(versions, engineversion.Version{
			Identity:      v.Identity,
			EngineVersion: v.EngineVersion,
			Enabled:       v.Enabled,
		})
	}
	return engineversion.SelectIdentity(postgresVersion, versions)
}

func (r *PostgresClusterReconciler) resolveEngineVersion(ctx context.Context, postgresVersion string) (string, error) {
	return r.resolveEngineVersionIdentity(ctx, postgresVersion)
}

func (r *PostgresClusterReconciler) setPostgresClusterErrorCondition(ctx context.Context, pg *dbaasv1.PostgresCluster, reason, message string, err error) (ctrl.Result, error) {
	SetStandardConditions(&pg.Status.Conditions, ConditionStateDegraded, reason, message)
	pg.Status.LastReconcileError = message
	if updateErr := r.updatePostgresClusterStatusWithRetry(ctx, pg); updateErr != nil {
		return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, updateErr
	}
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
}

func (r *PostgresClusterReconciler) updatePostgresClusterStatusWithRetry(ctx context.Context, pg *dbaasv1.PostgresCluster) error {
	return updateStatusWithRetry(func() error {
		var latest dbaasv1.PostgresCluster
		if err := r.Get(ctx, client.ObjectKeyFromObject(pg), &latest); err != nil {
			return err
		}
		latest.Status = pg.Status
		return r.Status().Update(ctx, &latest)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgresClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("postgrescluster")
	if r.Handler == nil {
		r.Handler = pgcluster.NewHandler(pgcluster.Config{
			Client:      r.Client,
			Scheme:      r.Scheme,
			DbaasClient: r.DbaasClient,
			IaaSClient:  r.IaaSClient,
			Recorder:    r.Recorder,
		})
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.PostgresCluster{}).
		Named("postgrescluster").
		Owns(&corev1.Service{}).
		Owns(&discoveryv1.EndpointSlice{}).
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
