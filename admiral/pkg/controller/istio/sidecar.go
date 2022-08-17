package istio

import (
	"context"
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

// SidecarHandler interface contains the methods that are required
type SidecarHandler interface {
	Added(ctx context.Context, obj *networking.Sidecar)
	Updated(ctx context.Context, obj *networking.Sidecar)
	Deleted(ctx context.Context, obj *networking.Sidecar)
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

func NewSidecarController(clusterID string, stopCh <-chan struct{}, handler SidecarHandler, config *rest.Config, resyncPeriod time.Duration) (*SidecarController, error) {

	sidecarController := SidecarController{}
	sidecarController.SidecarHandler = handler

	var err error

	ic, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sidecar controller k8s client: %v", err)
	}

	sidecarController.IstioClient = ic

	sidecarController.informer = informers.NewSidecarInformer(ic, k8sV1.NamespaceAll, resyncPeriod, cache.Indexers{})

	mcd := admiral.NewMonitoredDelegator(&sidecarController, clusterID, "sidecar")
	admiral.NewController("sidecar-ctrl-"+config.Host, stopCh, mcd, sidecarController.informer)

	return &sidecarController, nil
}

func (sec *SidecarController) Added(ctx context.Context, ojb interface{}) {
	sidecar := ojb.(*networking.Sidecar)
	sec.SidecarHandler.Added(ctx, sidecar)
}

func (sec *SidecarController) Updated(ctx context.Context, ojb interface{}, oldObj interface{}) {
	sidecar := ojb.(*networking.Sidecar)
	sec.SidecarHandler.Updated(ctx, sidecar)
}

func (sec *SidecarController) Deleted(ctx context.Context, ojb interface{}) {
	sidecar := ojb.(*networking.Sidecar)
	sec.SidecarHandler.Deleted(ctx, sidecar)

}
