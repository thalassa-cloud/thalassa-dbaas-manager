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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/thalassa-cloud/client-go/dbaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	pgref "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresclusterref"
	pgdatabase "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/postgresdatabase"
)

// PostgresDatabaseReconciler wires Kubernetes reconciliation to the Thalassa postgresdatabase.Handler.
type PostgresDatabaseReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	DbaasClient *dbaas.Client
	Recorder    record.EventRecorder
	Handler     *pgdatabase.Handler
}

// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresdatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresdatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresdatabases/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=postgresclusters,verbs=get;list;watch

// Reconcile reconciles a PostgresDatabase with the Thalassa DB cluster.
func (r *PostgresDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var db dbaasv1.PostgresDatabase
	if err := r.Get(ctx, req.NamespacedName, &db); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.DbaasClient == nil {
		log.Info("DBaaS client not configured, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if IsSuspended(&db) {
		return ctrl.Result{}, nil
	}

	db.Status.ObservedGeneration, db.Status.LastReconcileTime = ReconcileMeta(db.Generation)

	if !db.DeletionTimestamp.IsZero() {
		return r.Handler.Terminate(ctx, &db)
	}
	if controllerutil.AddFinalizer(&db, pgdatabase.Finalizer) {
		if err := r.Update(ctx, &db); err != nil {
			return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	clusterIdentity, err := pgref.Resolve(ctx, r.Client, db.Namespace, db.Spec.ClusterRef)
	if err != nil {
		if errors.Is(err, pgref.ErrDependencyNotReady) {
			return ctrl.Result{RequeueAfter: RequeueAfterDependencyNotReady}, nil
		}
		return r.Handler.SetErrorCondition(ctx, &db, "ClusterNotFound", err.Error(), err)
	}

	return r.Handler.Reconcile(ctx, pgdatabase.ReconcileInput{
		DB:              &db,
		ClusterIdentity: clusterIdentity,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgresDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("postgresdatabase") //nolint:staticcheck // SA1019: handlers use record.EventRecorder; events API uses different Eventf signature
	if r.Handler == nil {
		r.Handler = pgdatabase.NewHandler(pgdatabase.Config{
			Client:      r.Client,
			Scheme:      r.Scheme,
			DbaasClient: r.DbaasClient,
			Recorder:    r.Recorder,
		})
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.PostgresDatabase{}, builder.WithPredicates(PrimaryResourcePredicate())).
		Named("postgresdatabase").
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemFastSlowRateLimiter[reconcile.Request](1*time.Second, 10*time.Second, 15),
		}).
		Complete(r)
}
