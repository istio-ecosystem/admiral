package admiral

import (
	"context"
	"fmt"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	k8sV1Informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/rest"

	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// NodeHandler interface contains the methods that are required
type NodeHandler interface {
}

type NodeController struct {
	K8sClient   kubernetes.Interface
	NodeHandler NodeHandler
	Locality    *Locality
	informer    cache.SharedIndexInformer
}

type Locality struct {
	Region string
}

func NewNodeController(clusterID string, stopCh <-chan struct{}, handler NodeHandler, config *rest.Config) (*NodeController, error) {

	nodeController := NodeController{}
	nodeController.NodeHandler = handler

	var err error

	nodeController.K8sClient, err = K8sClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	nodeController.informer = k8sV1Informers.NewNodeInformer(
		nodeController.K8sClient,
		0,
		cache.Indexers{},
	)

	mcd := NewMonitoredDelegator(&nodeController, clusterID, "node")
	NewController("node-ctrl-"+config.Host, stopCh, mcd, nodeController.informer)

	return &nodeController, nil
}

func (p *NodeController) Added(ctx context.Context, obj interface{}) {
	node := obj.(*k8sV1.Node)
	if p.Locality == nil {
		p.Locality = &Locality{Region: common.GetNodeLocality(node)}
	}
}

func (p *NodeController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) {
	//ignore
}

func (p *NodeController) Deleted(ctx context.Context, obj interface{}) {
	//ignore
}
