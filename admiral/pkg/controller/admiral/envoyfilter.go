package admiral

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"

	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	log "github.com/sirupsen/logrus"
	networking "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioclientset "istio.io/client-go/pkg/clientset/versioned"
	networkingv1alpha3 "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// EnvoyFilterHandler interface contains the methods that are required
type EnvoyFilterHandler interface {
	Added(ctx context.Context, obj *networking.EnvoyFilter)
	Updated(ctx context.Context, obj *networking.EnvoyFilter)
	Deleted(ctx context.Context, obj *networking.EnvoyFilter)
}

type EnvoyFilterController struct {
	CrdClient          clientset.Interface
	IstioClient        istioclientset.Interface
	EnvoyFilterHandler EnvoyFilterHandler
	informer           cache.SharedIndexInformer
}

func (e *EnvoyFilterController) Added(ctx context.Context, obj interface{}) error {
	ef, ok := obj.(*networking.EnvoyFilter)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.EnvoyFilter", obj)
	}
	e.EnvoyFilterHandler.Added(ctx, ef)
	return nil
}

func (e *EnvoyFilterController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	ef, ok := obj.(*networking.EnvoyFilter)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.EnvoyFilter", obj)
	}
	e.EnvoyFilterHandler.Updated(ctx, ef)
	return nil
}

func (e *EnvoyFilterController) Deleted(ctx context.Context, obj interface{}) error {
	ef, ok := obj.(*networking.EnvoyFilter)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1alpha3.EnvoyFilter", obj)
	}
	e.EnvoyFilterHandler.Deleted(ctx, ef)
	return nil
}

func (d *EnvoyFilterController) GetProcessItemStatus(obj interface{}) (string, error) {
	return common.NotProcessed, nil
}

func (d *EnvoyFilterController) UpdateProcessItemStatus(obj interface{}, status string) error {
	return nil
}

func NewEnvoyFilterController(stopCh <-chan struct{}, handler EnvoyFilterHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*EnvoyFilterController, error) {
	envoyFilterController := EnvoyFilterController{}
	envoyFilterController.EnvoyFilterHandler = handler

	var err error

	envoyFilterController.CrdClient, err = clientLoader.LoadAdmiralClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create traffic config controller crd client: %v", err)
	}

	envoyFilterController.IstioClient, err = clientLoader.LoadIstioClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create traffic config controller crd client: %v", err)
	}

	envoyFilterController.informer = networkingv1alpha3.NewEnvoyFilterInformer(
		envoyFilterController.IstioClient,
		meta_v1.NamespaceAll, // TODO - change this to - admiral-sync namespace in future
		resyncPeriod,
		cache.Indexers{},
	)
	NewController("envoy-filter-ctrl", config.Host, stopCh, &envoyFilterController, envoyFilterController.informer)
	log.Debugln("NewEnvoyFilterController created....")
	return &envoyFilterController, nil
}

func (d *EnvoyFilterController) LogValueOfAdmiralIoIgnore(obj interface{}) {
}

func (d *EnvoyFilterController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*ef, ok := obj.(*networking.EnvoyFilter)
	if ok && d.IstioClient != nil {
		return d.IstioClient.NetworkingV1alpha3().EnvoyFilters(ef.Namespace).Get(ctx, ef.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("istio client is not initialized, txId=%s", ctx.Value("txId"))
}
