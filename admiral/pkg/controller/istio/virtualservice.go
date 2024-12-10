package istio

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// VirtualServiceHandler interface contains the methods that are required
type VirtualServiceHandler interface {
	Added(ctx context.Context, obj *networking.VirtualService) error
	Updated(ctx context.Context, obj *networking.VirtualService) error
	Deleted(ctx context.Context, obj *networking.VirtualService) error
}

type VirtualServiceController struct {
	IstioClient           versioned.Interface
	VirtualServiceHandler VirtualServiceHandler
	informer              cache.SharedIndexInformer
}

func (v *VirtualServiceController) DoesGenerationMatch(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func (v *VirtualServiceController) IsOnlyReplicaCountChanged(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func NewVirtualServiceController(stopCh <-chan struct{}, handler VirtualServiceHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*VirtualServiceController, error) {

	vsController := VirtualServiceController{}
	vsController.VirtualServiceHandler = handler

	var err error

	ic, err := clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtual service controller k8s client: %v", err)
	}

	vsController.IstioClient = ic
	vsController.informer = informers.NewVirtualServiceInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("virtualservice-ctrl", config.Host, stopCh, &vsController, vsController.informer)

	return &vsController, nil
}

func (sec *VirtualServiceController) Added(ctx context.Context, obj interface{}) error {
	dr, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return sec.VirtualServiceHandler.Added(ctx, dr)
}

func (sec *VirtualServiceController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	dr, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return sec.VirtualServiceHandler.Updated(ctx, dr)
}

func (sec *VirtualServiceController) Deleted(ctx context.Context, obj interface{}) error {
	dr, ok := obj.(*networking.VirtualService)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.VirtualService", obj)
	}
	return sec.VirtualServiceHandler.Deleted(ctx, dr)
}

func (sec *VirtualServiceController) GetProcessItemStatus(obj interface{}) (string, error) {
	return common.NotProcessed, nil
}

func (sec *VirtualServiceController) UpdateProcessItemStatus(obj interface{}, status string) error {
	return nil
}

func (sec *VirtualServiceController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	vs, ok := obj.(*networking.VirtualService)
	if !ok {
		return
	}
	if len(vs.Annotations) > 0 && vs.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", "VirtualService", vs.Name, vs.Namespace, "", "Value=true")
	}
}

func (sec *VirtualServiceController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*vs, ok := obj.(*networking.VirtualService)
	if ok && sec.IstioClient != nil {
		return sec.IstioClient.NetworkingV1alpha3().VirtualServices(vs.Namespace).Get(ctx, vs.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}
