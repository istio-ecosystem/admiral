package admiral

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
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

func (p *NodeController) DoesGenerationMatch(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

func (p *NodeController) IsOnlyReplicaCountChanged(*log.Entry, interface{}, interface{}) (bool, error) {
	return false, nil
}

type Locality struct {
	Region string
}

func NewNodeController(stopCh <-chan struct{}, handler NodeHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*NodeController, error) {

	nodeController := NodeController{}
	nodeController.NodeHandler = handler

	var err error

	nodeController.K8sClient, err = clientLoader.LoadKubeClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependency controller k8s client: %v", err)
	}

	nodeController.informer = k8sV1Informers.NewNodeInformer(
		nodeController.K8sClient,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("node-ctrl", config.Host, stopCh, &nodeController, nodeController.informer)

	return &nodeController, nil
}

func process(ctx context.Context, obj interface{}) (string, error) {
	node, ok := obj.(*k8sV1.Node)
	if !ok {
		return "", fmt.Errorf("type assertion failed, %v is not of type *v1.Node", obj)
	}
	return common.GetNodeLocality(node), nil
}

func (d *NodeController) Added(ctx context.Context, obj interface{}) error {
	region, err := process(ctx, obj)
	if err != nil {
		return err
	}
	if region == "" {
		log.Debugf("received empty region for node %v", obj)
		return nil
	}
	d.Locality = &Locality{Region: region}
	return nil
}

func (d *NodeController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	region, err := process(ctx, obj)
	if err != nil {
		return err
	}
	if region == "" {
		log.Debugf("received empty region for node %v", obj)
		return nil
	}
	d.Locality = &Locality{Region: region}
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
