package istio

import (
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Handler interface contains the methods that are required
type VirtualServiceHandler interface {
	Added(obj *networking.VirtualService)
	Updated(obj *networking.VirtualService)
	Deleted(obj *networking.VirtualService)
}

type VirtualServiceController struct {
	IstioClient           versioned.Interface
	VirtualServiceHandler VirtualServiceHandler
	informer              cache.SharedIndexInformer
}

func NewVirtualServiceController(stopCh <-chan struct{}, handler VirtualServiceHandler, config *rest.Config, resyncPeriod time.Duration) (*VirtualServiceController, error) {

	drController := VirtualServiceController{}
	drController.VirtualServiceHandler = handler

	var err error

	ic, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create virtual service controller k8s client: %v", err)
	}

	drController.IstioClient = ic

	drController.informer = informers.NewVirtualServiceInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("virtualservice-ctrl-" + config.Host, stopCh, &drController, drController.informer)

	return &drController, nil
}

func (sec *VirtualServiceController) Added(ojb interface{}) {
	dr := ojb.(*networking.VirtualService)
	sec.VirtualServiceHandler.Added(dr)
}

func (sec *VirtualServiceController) Updated(ojb interface{}, oldObj interface{}) {
	dr := ojb.(*networking.VirtualService)
	sec.VirtualServiceHandler.Updated(dr)
}

func (sec *VirtualServiceController) Deleted(ojb interface{}) {
	dr := ojb.(*networking.VirtualService)
	sec.VirtualServiceHandler.Deleted(dr)

}
