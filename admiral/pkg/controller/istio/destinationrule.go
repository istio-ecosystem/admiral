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
type DestinationRuleHandler interface {
	Added(obj *networking.DestinationRule)
	Updated(obj *networking.DestinationRule)
	Deleted(obj *networking.DestinationRule)
}

type DestinationRuleEntry struct {
	Identity        string
	DestinationRule *networking.DestinationRule
}

type DestinationRuleController struct {
	IstioClient            versioned.Interface
	DestinationRuleHandler DestinationRuleHandler
	informer               cache.SharedIndexInformer
}

func NewDestinationRuleController(stopCh <-chan struct{}, handler DestinationRuleHandler, config *rest.Config, resyncPeriod time.Duration) (*DestinationRuleController, error) {

	drController := DestinationRuleController{}
	drController.DestinationRuleHandler = handler

	var err error

	ic, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination rule controller k8s client: %v", err)
	}

	drController.IstioClient = ic

	drController.informer = informers.NewDestinationRuleInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("destinationrule-ctrl-" + config.Host, stopCh, &drController, drController.informer)

	return &drController, nil
}

func (sec *DestinationRuleController) Added(ojb interface{}) {
	dr := ojb.(*networking.DestinationRule)
	sec.DestinationRuleHandler.Added(dr)
}

func (sec *DestinationRuleController) Updated(ojb interface{}, oldObj interface{}) {
	dr := ojb.(*networking.DestinationRule)
	sec.DestinationRuleHandler.Updated(dr)
}

func (sec *DestinationRuleController) Deleted(ojb interface{}) {
	dr := ojb.(*networking.DestinationRule)
	sec.DestinationRuleHandler.Deleted(dr)

}
