package istio

import (
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	versioned "istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Handler interface contains the methods that are required
type ServiceEntryHandler interface {
	Added(obj *networking.ServiceEntry)
	Updated(obj *networking.ServiceEntry)
	Deleted(obj *networking.ServiceEntry)
}

type ServiceEntryEntry struct {
	Identity     string
	ServiceEntry *networking.ServiceEntry
}

type ServiceEntryController struct {
	IstioClient         versioned.Interface
	ServiceEntryHandler ServiceEntryHandler
	informer            cache.SharedIndexInformer
}

func NewServiceEntryController(stopCh <-chan struct{}, handler ServiceEntryHandler, config *rest.Config, resyncPeriod time.Duration) (*ServiceEntryController, error) {

	seController := ServiceEntryController{}
	seController.ServiceEntryHandler = handler

	var err error

	ic, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create service entry k8s client: %v", err)
	}

	seController.IstioClient = ic

	seController.informer = informers.NewServiceEntryInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("serviceentry-ctrl-" + config.Host, stopCh, &seController, seController.informer)

	return &seController, nil
}

func (sec *ServiceEntryController) Added(ojb interface{}) {
	se := ojb.(*networking.ServiceEntry)
	sec.ServiceEntryHandler.Added(se)
}

func (sec *ServiceEntryController) Updated(ojb interface{}, oldObj interface{}) {
	se := ojb.(*networking.ServiceEntry)
	sec.ServiceEntryHandler.Updated(se)
}

func (sec *ServiceEntryController) Deleted(ojb interface{}) {
	se := ojb.(*networking.ServiceEntry)
	sec.ServiceEntryHandler.Deleted(se)

}
