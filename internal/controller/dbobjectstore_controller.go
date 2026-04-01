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
	"time"

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
	dbobjectstore "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/dbobjectstore"
)

// DbObjectStoreReconciler wires Kubernetes reconciliation to the Thalassa dbobjectstore.Handler.
type DbObjectStoreReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	DbaasClient     *dbaas.Client
	IaaSClient      *thalassaiaas.Client
	DefaultRegion   string
	DefaultSubnetID string
	Recorder        record.EventRecorder
	Handler         *dbobjectstore.Handler
}

// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=dbobjectstores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=dbobjectstores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbaas.controllers.thalassa.cloud,resources=dbobjectstores/finalizers,verbs=update

// Reconcile reconciles a DbObjectStore.
func (r *DbObjectStoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var obj dbaasv1.DbObjectStore
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.DbaasClient == nil {
		log.Info("DBaaS client not configured, skipping reconciliation")
		return ctrl.Result{}, nil
	}
	if IsSuspended(&obj) {
		return ctrl.Result{}, nil
	}

	obj.Status.ObservedGeneration, obj.Status.LastReconcileTime = ReconcileMeta(obj.Generation)

	if !obj.DeletionTimestamp.IsZero() {
		return r.Handler.Terminate(ctx, &obj)
	}
	if controllerutil.AddFinalizer(&obj, dbobjectstore.Finalizer) {
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{RequeueAfter: RequeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	return r.Handler.Reconcile(ctx, dbobjectstore.ReconcileInput{Obj: &obj})
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbObjectStoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("dbobjectstore")
	if r.Handler == nil {
		r.Handler = dbobjectstore.NewHandler(dbobjectstore.Config{
			Client:          r.Client,
			Scheme:          r.Scheme,
			DbaasClient:     r.DbaasClient,
			IaaSClient:      r.IaaSClient,
			DefaultRegion:   r.DefaultRegion,
			DefaultSubnetID: r.DefaultSubnetID,
			Recorder:        r.Recorder,
		})
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.DbObjectStore{}).
		Named("dbobjectstore").
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemFastSlowRateLimiter[reconcile.Request](time.Second, 10*time.Second, 15),
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
