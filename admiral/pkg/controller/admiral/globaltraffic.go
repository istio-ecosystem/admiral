package admiral

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"time"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1"
)

// Handler interface contains the methods that are required
type GlobalTrafficHandler interface {
	Added(obj *v1.GlobalTrafficPolicy)
	Updated(obj *v1.GlobalTrafficPolicy)
	Deleted(obj *v1.GlobalTrafficPolicy)
}

type GlobalTrafficController struct {
	CrdClient            clientset.Interface
	GlobalTrafficHandler GlobalTrafficHandler
	informer             cache.SharedIndexInformer
}

func NewGlobalTrafficController(stopCh <-chan struct{}, handler GlobalTrafficHandler, configPath *rest.Config, resyncPeriod time.Duration) (*GlobalTrafficController, error) {

	globalTrafficController := GlobalTrafficController{}

	globalTrafficController.GlobalTrafficHandler = handler

	var err error

	globalTrafficController.CrdClient, err = AdmiralCrdClientFromConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create global traffic controller crd client: %v", err)
	}

	globalTrafficController.informer = informerV1.NewGlobalTrafficPolicyInformer(
		globalTrafficController.CrdClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("gtp-ctrl-" + configPath.Host, stopCh, &globalTrafficController, globalTrafficController.informer)

	return &globalTrafficController, nil
}

func (d *GlobalTrafficController) Added(ojb interface{}) {
	dep := ojb.(*v1.GlobalTrafficPolicy)
	d.GlobalTrafficHandler.Added(dep)
}

func (d *GlobalTrafficController) Updated(ojb interface{}, oldObj interface{}) {
	dep := ojb.(*v1.GlobalTrafficPolicy)
	d.GlobalTrafficHandler.Updated(dep)
}

func (d *GlobalTrafficController) Deleted(ojb interface{}) {
	dep := ojb.(*v1.GlobalTrafficPolicy)
	d.GlobalTrafficHandler.Deleted(dep)
}

func (g *GlobalTrafficController) GetGTPByLabel(labelValue string, namespace string) []v1.GlobalTrafficPolicy {
	matchLabel := common.GetGlobalTrafficDeploymentLabel()
	labelOptions := meta_v1.ListOptions{}
	labelOptions.LabelSelector = fmt.Sprintf("%s=%s", matchLabel, labelValue)
	matchedDeployments, err := g.CrdClient.AdmiralV1().GlobalTrafficPolicies(namespace).List(labelOptions)

	if err != nil {
		logrus.Errorf("Failed to list GTPs in cluster, error: %v", err)
		return nil
	}

	if matchedDeployments.Items == nil {
		return []v1.GlobalTrafficPolicy{}
	}

	return matchedDeployments.Items
}
