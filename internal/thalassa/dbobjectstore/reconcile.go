package dbobjectstore

import (
	"context"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thalassa-cloud/client-go/dbaas"
	thalassaclient "github.com/thalassa-cloud/client-go/pkg/client"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	stdconditions "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/conditions"
	helpers "github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassa/helpers"
)

func (h *Handler) createObjectStore(ctx context.Context, obj *dbaasv1.DbObjectStore, region string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Creating", "Creating DB object store in Thalassa")
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}

	createReq := dbaas.CreateDbObjectStoreRequest{
		Name:             helpers.EffectiveName(obj.Name, obj.Spec.Metadata),
		Description:      obj.Spec.Description,
		Labels:           dbaas.Labels(helpers.EffectiveLabelsDbaas(obj.Spec.Metadata)),
		Annotations:      dbaas.Annotations(helpers.EffectiveAnnotationsDbaas(obj.Spec.Metadata)),
		Region:           region,
		RetentionPolicy:  obj.Spec.RetentionPolicy,
		DeleteProtection: obj.Spec.DeleteProtection,
	}

	created, err := h.DbaasClient.CreateDbObjectStore(ctx, createReq)
	if err != nil {
		return h.setErrorCondition(ctx, obj, "FailedCreate", err.Error(), err)
	}

	h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Created", "Created DB object store in Thalassa (%s)", created.Identity)

	obj.Status.ResourceID = created.Identity
	obj.Status.ResourceStatus = string(created.Status)
	obj.Status.StatusMessage = created.StatusMessage

	if created.ServiceAccount != nil {
		obj.Status.ServiceAccount = created.ServiceAccount.Identity
	}
	if created.ObjectStorageBucket != nil {
		obj.Status.ObjectStorageBucket = created.ObjectStorageBucket.Identity
		obj.Status.ObjectStorageBucketUsage = dbaasv1.ObjectStorageBucketUsage{
			TotalSize:    helpers.QuantityFromDecimalGigabytes(created.ObjectStorageBucket.Usage.TotalSizeGB),
			TotalObjects: created.ObjectStorageBucket.Usage.TotalObjects,
		}
	}
	if created.Region != nil {
		obj.Status.Region = created.Region.Slug
	}
	obj.Status.LastReconcileError = ""
	h.setConditionFromProviderStatus(obj, string(created.Status), created.StatusMessage)
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}
	log.Info("created DB object store in Thalassa", "identity", created.Identity)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (h *Handler) reconcileObjectStore(ctx context.Context, obj *dbaasv1.DbObjectStore) (ctrl.Result, error) {
	identity := obj.Status.ResourceID
	fetched, err := h.DbaasClient.GetDbObjectStore(ctx, identity)
	if err != nil {
		if thalassaclient.IsNotFound(err) {
			obj.Status.ResourceID = ""
			if err := h.updateStatusWithRetry(ctx, obj); err != nil {
				return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
			}
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
		return h.setErrorCondition(ctx, obj, "GetFailed", err.Error(), err)
	}

	obj.Status.ResourceStatus = string(fetched.Status)
	obj.Status.StatusMessage = fetched.StatusMessage
	if fetched.Region != nil {
		obj.Status.Region = fetched.Region.Slug
	}
	if fetched.ServiceAccount != nil {
		obj.Status.ServiceAccount = fetched.ServiceAccount.Identity
	}
	if fetched.ObjectStorageBucket != nil {
		obj.Status.ObjectStorageBucket = fetched.ObjectStorageBucket.Identity
		obj.Status.ObjectStorageBucketUsage = dbaasv1.ObjectStorageBucketUsage{
			TotalSize:    helpers.QuantityFromDecimalGigabytes(fetched.ObjectStorageBucket.Usage.TotalSizeGB),
			TotalObjects: fetched.ObjectStorageBucket.Usage.TotalObjects,
		}
	}
	obj.Status.LastReconcileError = ""

	if h.objectStoreRequiresUpdate(obj, fetched) {
		upd := dbaas.UpdateDbObjectStoreRequest{
			Name:             helpers.EffectiveName(obj.Name, obj.Spec.Metadata),
			Description:      obj.Spec.Description,
			Labels:           dbaas.Labels(helpers.EffectiveLabelsDbaas(obj.Spec.Metadata)),
			Annotations:      dbaas.Annotations(helpers.EffectiveAnnotationsDbaas(obj.Spec.Metadata)),
			RetentionPolicy:  obj.Spec.RetentionPolicy,
			DeleteProtection: obj.Spec.DeleteProtection,
		}
		if _, err := h.DbaasClient.UpdateDbObjectStore(ctx, identity, upd); err != nil {
			return h.setErrorCondition(ctx, obj, "FailedUpdate", err.Error(), err)
		}
		h.Recorder.Eventf(obj, corev1.EventTypeNormal, "Updated", "Updated DB object store in Thalassa (%s)", identity)
		stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Updating", "Updating DB object store in Thalassa")
		if err := h.updateStatusWithRetry(ctx, obj); err != nil {
			return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	h.setConditionFromProviderStatus(obj, string(fetched.Status), fetched.StatusMessage)
	if err := h.updateStatusWithRetry(ctx, obj); err != nil {
		return ctrl.Result{RequeueAfter: requeueAfterStatusUpdateFailure}, err
	}

	requeue := 5 * time.Minute
	if !strings.EqualFold(string(fetched.Status), string(dbaas.ObjectStatusReady)) {
		requeue = 20 * time.Second
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (h *Handler) objectStoreRequiresUpdate(obj *dbaasv1.DbObjectStore, fetched *dbaas.DbObjectStore) bool {
	if helpers.EffectiveName(obj.Name, obj.Spec.Metadata) != fetched.Name {
		return true
	}
	if obj.Spec.Description != fetched.Description {
		return true
	}
	if obj.Spec.DeleteProtection != fetched.DeleteProtection {
		return true
	}
	if obj.Spec.RetentionPolicy != fetched.RetentionPolicy {
		return true
	}
	wantLabels := map[string]string(helpers.EffectiveLabelsDbaas(obj.Spec.Metadata))
	if wantLabels == nil {
		wantLabels = map[string]string{}
	}
	gotLabels := map[string]string(fetched.Labels)
	if !equality.Semantic.DeepEqual(wantLabels, gotLabels) {
		return true
	}
	return false
}

func (h *Handler) setConditionFromProviderStatus(obj *dbaasv1.DbObjectStore, status, msg string) {
	switch {
	case strings.EqualFold(status, string(dbaas.ObjectStatusReady)):
		stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateAvailable, "Ready", firstNonEmpty(msg, "DB object store is ready"))
	case strings.EqualFold(status, string(dbaas.ObjectStatusFailed)),
		strings.EqualFold(status, string(dbaas.ObjectStatusDeleted)):
		stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateDegraded, "NotReady", firstNonEmpty(msg, "status: "+status))
	default:
		stdconditions.SetStandardConditions(&obj.Status.Conditions, stdconditions.ConditionStateProgressing, "Provisioning", firstNonEmpty(msg, "status: "+status))
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
