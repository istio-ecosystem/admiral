
package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"time"
)

// Handler interface contains the methods that are required
type RoutingPolicyHandler interface {
	Added(obj *v1.RoutingPolicy)
	Updated(obj *v1.RoutingPolicy)
	Deleted(obj *v1.RoutingPolicy)
}

type RoutingPolicyEntry struct {
	Identity string
	RoutingPolicy  *v1.RoutingPolicy
}

type RoutingPolicyClusterEntry struct {
	Identity string
	RoutingPolicies map[string]*v1.RoutingPolicy
}

type RoutingPolicyController struct {
	CrdClient            clientset.Interface
	RoutingPolicyHandler RoutingPolicyHandler
	informer             cache.SharedIndexInformer
}

func (r *RoutingPolicyController) Added(obj interface{}) {
	routingPolicy := obj.(*v1.RoutingPolicy)
	r.RoutingPolicyHandler.Added(routingPolicy)
}

func (r *RoutingPolicyController) Updated(obj interface{}, oldObj interface{}) {
	routingPolicy := obj.(*v1.RoutingPolicy)
	r.RoutingPolicyHandler.Updated(routingPolicy)
}

func (r *RoutingPolicyController) Deleted(obj interface{}) {
	routingPolicy := obj.(*v1.RoutingPolicy)
	r.RoutingPolicyHandler.Deleted(routingPolicy)
}

func NewRoutingPoliciesController(stopCh <-chan struct{}, handler RoutingPolicyHandler, configPath *rest.Config, resyncPeriod time.Duration) (*RoutingPolicyController, error) {

	rpController := RoutingPolicyController{}
	rpController.RoutingPolicyHandler = handler

	var err error

	rpController.CrdClient, err = AdmiralCrdClientFromConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing policy controller crd client: %v", err)
	}

	rpController.informer = informerV1.NewRoutingPolicyInformer(
		rpController.CrdClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController(stopCh, &rpController, rpController.informer)

	return &rpController, nil

}
