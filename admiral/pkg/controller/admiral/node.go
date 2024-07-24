package admiral

import (
	"context"
	"fmt"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
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

func NewNodeController(stopCh <-chan struct{}, handler NodeHandler, config *rest.Config, clientLoader loader.ClientLoader) (*NodeController, error) {

	nodeController := NodeController{}
	nodeController.NodeHandler = handler

	var err error

	nodeController.K8sClient, err = clientLoader.LoadKubeClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	nodeController.informer = k8sV1Informers.NewNodeInformer(
		nodeController.K8sClient,
		0,
		cache.Indexers{},
	)

	NewController("node-ctrl", config.Host, stopCh, &nodeController, nodeController.informer)

	return &nodeController, nil
}

func (p *NodeController) Added(ctx context.Context, obj interface{}) error {
	node, ok := obj.(*k8sV1.Node)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Node", obj)
	}
	if p.Locality == nil {
		p.Locality = &Locality{Region: common.GetNodeLocality(node)}
	}
	return nil
}

func (p *NodeController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	//ignore
	return nil
}

func (p *NodeController) Deleted(ctx context.Context, obj interface{}) error {
	//ignore
	return nil
}

func (d *NodeController) GetProcessItemStatus(obj interface{}) (string, error) {
	return common.NotProcessed, nil
}

func (d *NodeController) UpdateProcessItemStatus(obj interface{}, status string) error {
	return nil
}

func (d *NodeController) LogValueOfAdmiralIoIgnore(obj interface{}) {
}

func (d *NodeController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*node, ok := obj.(*k8sV1.Node)
	if ok && d.K8sClient != nil {
		return d.K8sClient.CoreV1().Nodes().Get(ctx, node.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}
