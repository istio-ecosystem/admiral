package istio

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	versioned "istio.io/client-go/pkg/clientset/versioned"
	informers "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// SidecarHandler interface contains the methods that are required
type SidecarHandler interface {
	Added(ctx context.Context, obj *networking.Sidecar) error
	Updated(ctx context.Context, obj *networking.Sidecar) error
	Deleted(ctx context.Context, obj *networking.Sidecar) error
}

type SidecarEntry struct {
	Identity string
	Sidecar  *networking.Sidecar
}

type SidecarController struct {
	IstioClient    versioned.Interface
	SidecarHandler SidecarHandler
	informer       cache.SharedIndexInformer
}

func NewSidecarController(stopCh <-chan struct{}, handler SidecarHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*SidecarController, error) {

	sidecarController := SidecarController{}
	sidecarController.SidecarHandler = handler

	var err error

	ic, err := clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sidecar controller k8s client: %v", err)
	}

	sidecarController.IstioClient = ic

	sidecarController.informer = informers.NewSidecarInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	admiral.NewController("sidecar-ctrl", config.Host, stopCh, &sidecarController, sidecarController.informer)

	return &sidecarController, nil
}

func (sec *SidecarController) Added(ctx context.Context, obj interface{}) error {
	sidecar, ok := obj.(*networking.Sidecar)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.Sidecar", obj)
	}
	return sec.SidecarHandler.Added(ctx, sidecar)
}

func (sec *SidecarController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	sidecar, ok := obj.(*networking.Sidecar)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.Sidecar", obj)
	}
	return sec.SidecarHandler.Updated(ctx, sidecar)
}

func (sec *SidecarController) Deleted(ctx context.Context, obj interface{}) error {
	sidecar, ok := obj.(*networking.Sidecar)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.Sidecar", obj)
	}
	return sec.SidecarHandler.Deleted(ctx, sidecar)
}

func (sec *SidecarController) GetProcessItemStatus(obj interface{}) (string, error) {
	return common.NotProcessed, nil
}

func (sec *SidecarController) UpdateProcessItemStatus(obj interface{}, status string) error {
	return nil
}

func (sec *SidecarController) LogValueOfAdmiralIoIgnore(obj interface{}) {
}

func (sec *SidecarController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*sidecar, ok := obj.(*networking.Sidecar)
	if ok && sec.IstioClient != nil {
		return sec.IstioClient.NetworkingV1alpha3().Sidecars(sidecar.Namespace).Get(ctx, sidecar.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}
