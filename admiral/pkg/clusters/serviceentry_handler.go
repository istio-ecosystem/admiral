package clusters

import (
	"bytes"
	"context"
	"fmt"
	"time"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceEntryHandler responsible for handling Add/Update/Delete events for
// ServiceEntry resources
type ServiceEntryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (se *ServiceEntryHandler) Added(obj *v1alpha3.ServiceEntry) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, "Add", "ServiceEntry", obj.Name, se.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Add", "ServiceEntry", obj.Name, se.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		if len(obj.Annotations) > 0 && obj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof(LogFormat, "admiralIoIgnoreAnnotationCheck", "ServiceEntry", obj.Name, se.ClusterID, "Value=true namespace="+obj.Namespace)
		}
	}
	return nil
}

func (se *ServiceEntryHandler) Updated(obj *v1alpha3.ServiceEntry) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, "Update", "ServiceEntry", obj.Name, se.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Update", "ServiceEntry", obj.Name, se.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		if len(obj.Annotations) > 0 && obj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof(LogFormat, "admiralIoIgnoreAnnotationCheck", "ServiceEntry", obj.Name, se.ClusterID, "Value=true namespace="+obj.Namespace)
		}
	}
	return nil
}

func (se *ServiceEntryHandler) Deleted(obj *v1alpha3.ServiceEntry) error {
	if commonUtil.IsAdmiralReadOnly() {
		log.Infof(LogFormat, "Delete", "ServiceEntry", obj.Name, se.ClusterID, "Admiral is in read-only mode. Skipping resource from namespace="+obj.Namespace)
		return nil
	}
	if IgnoreIstioResource(obj.Spec.ExportTo, obj.Annotations, obj.Namespace) {
		log.Infof(LogFormat, "Delete", "ServiceEntry", obj.Name, se.ClusterID, "Skipping resource from namespace="+obj.Namespace)
		if len(obj.Annotations) > 0 && obj.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Debugf(LogFormat, "admiralIoIgnoreAnnotationCheck", "ServiceEntry", obj.Name, se.ClusterID, "Value=true namespace="+obj.Namespace)
		}
	}
	return nil
}

/*
Add/Update Service Entry objects after checking if the current pod is in ReadOnly mode.
Service Entry object is not added/updated if the current pod is in ReadOnly mode.
*/
func addUpdateServiceEntry(ctxLogger *log.Entry, ctx context.Context,
	obj *v1alpha3.ServiceEntry, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) error {
	var (
		err             error
		op, diff        string
		skipUpdate      bool
		seAlreadyExists bool
	)
	ctxLogger.Infof(common.CtxLogFormat, "AddUpdateServiceEntry", "", "", rc.ClusterID, "Creating/Updating ServiceEntry="+obj.Name)
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations["app.kubernetes.io/created-by"] = "admiral"

	areEndpointsValid := validateAndProcessServiceEntryEndpoints(obj)

	seIsNew := exist == nil || exist.Spec.Hosts == nil
	if seIsNew {
		op = "Add"
		//se will be created if endpoints are valid, in case they are not valid se will be created with just valid endpoints
		if len(obj.Spec.Endpoints) > 0 {
			obj.Namespace = namespace
			obj.ResourceVersion = ""
			_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(ctx, obj, metav1.CreateOptions{})
			if k8sErrors.IsAlreadyExists(err) {
				// op=%v name=%v namespace=%s cluster=%s message=%v
				ctxLogger.Infof(common.CtxLogFormat, "addUpdateServiceEntry", obj.Name, obj.Namespace, rc.ClusterID, "object already exists. Will update instead")
				seAlreadyExists = true
			} else {
				return err
			}
			ctxLogger.Infof(common.CtxLogFormat, "Add", " SE=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "New SE", obj.Spec.String())
		} else {
			log.Errorf(LogFormat+" SE=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "Creation of SE skipped as endpoints are not valid", obj.Spec.String())
		}
	}
	if !seIsNew || seAlreadyExists {
		if seAlreadyExists {
			exist, err = rc.ServiceEntryController.IstioClient.
				NetworkingV1alpha3().
				ServiceEntries(namespace).
				Get(ctx, obj.Name, metav1.GetOptions{})
			if err != nil {
				exist = obj
				// when there is an error, assign exist to obj,
				// which will fail in the update operation, but will be retried
				// in the retry logic
				ctxLogger.Warnf(common.CtxLogFormat, "Update", exist.Name, exist.Namespace, rc.ClusterID, "got error on fetching se, will retry updating")
			}
		}
		op = "Update"
		if areEndpointsValid { //update will happen only when all the endpoints are valid // TODO: why not have this check when
			exist.Labels = obj.Labels
			exist.Annotations = obj.Annotations
			skipUpdate, diff = skipDestructiveUpdate(rc, obj, exist)
			if diff != "" {
				ctxLogger.Infof(LogFormat+" diff=%s", op, "ServiceEntry", obj.Name, rc.ClusterID, "Diff in update", diff)
			}
			if skipUpdate {
				ctxLogger.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Update skipped as it was destructive during Admiral's bootup phase")
				return nil
			} else {
				//nolint
				exist.Spec = obj.Spec
				_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Update(ctx, exist, metav1.UpdateOptions{})
				if err != nil {
					err = retryUpdatingSE(ctxLogger, ctx, obj, exist, namespace, rc, err, op)
				}
			}
		} else {
			ctxLogger.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "SE could not be updated as all the recived endpoints are not valid.")
		}

	}

	if err != nil {
		ctxLogger.Errorf(LogErrFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, err)
		return err
	} else {
		ctxLogger.Infof(LogFormat, op, "ServiceEntry", obj.Name, rc.ClusterID, "Success")
	}
	return nil
}

func retryUpdatingSE(ctxLogger *log.Entry, ctx context.Context, obj *v1alpha3.ServiceEntry, exist *v1alpha3.ServiceEntry, namespace string, rc *RemoteController, err error, op string) error {
	numRetries := 5
	if err != nil && k8sErrors.IsConflict(err) {
		for i := 0; i < numRetries; i++ {
			ctxLogger.Errorf(common.CtxLogFormat, op, obj.Name, obj.Namespace, rc.ClusterID, err.Error()+". will retry the update operation before adding back to the controller queue.")

			updatedServiceEntry, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(common.GetSyncNamespace()).Get(ctx, exist.Name, metav1.GetOptions{})
			// if old service entry not find, just create a new service entry instead
			if err != nil {
				ctxLogger.Infof(common.CtxLogFormat, op, exist.Name, exist.Namespace, rc.ClusterID, err.Error()+fmt.Sprintf(". Error getting old serviceEntry"))
				continue
			}

			ctxLogger.Infof(common.CtxLogFormat, op, obj.Name, obj.Namespace, rc.ClusterID, fmt.Sprintf("existingResourceVersion=%s resourceVersionUsedForUpdate=%s", updatedServiceEntry.ResourceVersion, obj.ResourceVersion))
			updatedServiceEntry.Spec = obj.Spec
			updatedServiceEntry.Annotations = obj.Annotations
			updatedServiceEntry.Labels = obj.Labels
			_, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Update(ctx, updatedServiceEntry, metav1.UpdateOptions{})
			if err == nil {
				return nil
			}
		}
	}
	return err
}

func skipDestructiveUpdate(rc *RemoteController, new *v1alpha3.ServiceEntry, old *v1alpha3.ServiceEntry) (bool, string) {
	var (
		skipDestructive   = false
		destructive, diff = getServiceEntryDiff(new, old)
	)

	//do not update SEs during bootup phase if they are destructive
	if time.Since(rc.StartTime) < (2*common.GetAdmiralParams().CacheReconcileDuration) && destructive {
		skipDestructive = true
	}
	return skipDestructive, diff
}

// Diffs only endpoints
func getServiceEntryDiff(new *v1alpha3.ServiceEntry, old *v1alpha3.ServiceEntry) (destructive bool, diff string) {
	//we diff only if both objects exist
	if old == nil || new == nil {
		return false, ""
	}
	destructive = false
	format := "%s %s before: %v, after: %v;"
	var buffer bytes.Buffer
	//nolint
	seNew := new.Spec
	//nolint
	seOld := old.Spec

	oldEndpointMap := make(map[string]*networkingv1alpha3.WorkloadEntry)
	found := make(map[string]string)
	for _, oEndpoint := range seOld.Endpoints {
		oldEndpointMap[oEndpoint.Address] = oEndpoint
	}
	for _, nEndpoint := range seNew.Endpoints {
		if val, ok := oldEndpointMap[nEndpoint.Address]; ok {
			found[nEndpoint.Address] = "1"
			if val.String() != nEndpoint.String() {
				destructive = true
				buffer.WriteString(fmt.Sprintf(format, "endpoint", "Update", val.String(), nEndpoint.String()))
			}
		} else {
			buffer.WriteString(fmt.Sprintf(format, "endpoint", "Add", "", nEndpoint.String()))
		}
	}

	for key := range oldEndpointMap {
		if _, ok := found[key]; !ok {
			destructive = true
			buffer.WriteString(fmt.Sprintf(format, "endpoint", "Delete", oldEndpointMap[key].String(), ""))
		}
	}

	if common.EnableExportTo(seNew.Hosts[0]) {
		oldNamespacesMap := make(map[string]struct{})
		for _, oldNamespace := range seOld.ExportTo {
			oldNamespacesMap[oldNamespace] = struct{}{}
		}
		//If new NS was not in old NS map then it was added non-destructively
		//If new NS was in old NS map then there is no problem, and we remove it from old NS map
		for _, newNamespace := range seNew.ExportTo {
			if _, ok := oldNamespacesMap[newNamespace]; !ok {
				buffer.WriteString(fmt.Sprintf(format, "exportTo namespace", "Add", "", newNamespace))
			} else {
				delete(oldNamespacesMap, newNamespace)
			}
		}
		//Old NS map only contains namespaces that weren't present in new NS slice because we removed all the ones that were present in both
		//If old NS isn't in the new NS map, then it was deleted destructively
		for key := range oldNamespacesMap {
			destructive = true
			buffer.WriteString(fmt.Sprintf(format, "exportTo namespace", "Delete", key, ""))
		}
	}

	diff = buffer.String()
	return destructive, diff
}

func deleteServiceEntry(ctx context.Context, serviceEntry *v1alpha3.ServiceEntry, namespace string, rc *RemoteController) error {
	if serviceEntry != nil {
		err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Delete(ctx, serviceEntry.Name, metav1.DeleteOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				log.Infof(LogFormat, "Delete", "ServiceEntry", serviceEntry.Name, rc.ClusterID, "Either ServiceEntry was already deleted, or it never existed")
			} else {
				log.Errorf(LogErrFormat, "Delete", "ServiceEntry", serviceEntry.Name, rc.ClusterID, err)
				return err
			}
		} else {
			log.Infof(LogFormat, "Delete", "ServiceEntry", serviceEntry.Name, rc.ClusterID, "Success")
		}
	}
	return nil
}

// nolint
func createSidecarSkeleton(sidecar networkingv1alpha3.Sidecar, name string, namespace string) *v1alpha3.Sidecar {
	return &v1alpha3.Sidecar{Spec: sidecar, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

func validateAndProcessServiceEntryEndpoints(obj *v1alpha3.ServiceEntry) bool {
	var areEndpointsValid = true

	temp := make([]*networkingv1alpha3.WorkloadEntry, 0)
	for _, endpoint := range obj.Spec.Endpoints {
		if endpoint.Address == "dummy.admiral.global" {
			areEndpointsValid = false
		} else {
			temp = append(temp, endpoint)
		}
	}
	obj.Spec.Endpoints = temp
	log.Infof("type=ServiceEntry, name=%s, endpointsValid=%v, numberOfValidEndpoints=%d", obj.Name, areEndpointsValid, len(obj.Spec.Endpoints))

	return areEndpointsValid
}

// nolint
func createServiceEntrySkeleton(se networkingv1alpha3.ServiceEntry, name string, namespace string) *v1alpha3.ServiceEntry {
	return &v1alpha3.ServiceEntry{Spec: se, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}
