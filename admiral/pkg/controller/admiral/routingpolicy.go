package admiral

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	informerV1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/informers/externalversions/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"istio.io/client-go/pkg/clientset/versioned"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// RoutingPolicyHandler interface contains the methods that are required
type RoutingPolicyHandler interface {
	Added(ctx context.Context, obj *v1.RoutingPolicy) error
	Updated(ctx context.Context, obj *v1.RoutingPolicy) error
	Deleted(ctx context.Context, obj *v1.RoutingPolicy) error
}

type RoutingPolicyEntry struct {
	Identity      string
	RoutingPolicy *v1.RoutingPolicy
}

type RoutingPolicyClusterEntry struct {
	Identity        string
	RoutingPolicies map[string]*v1.RoutingPolicy
}

type RoutingPolicyController struct {
	K8sClient            kubernetes.Interface
	CrdClient            clientset.Interface
	IstioClient          versioned.Interface
	RoutingPolicyHandler RoutingPolicyHandler
	informer             cache.SharedIndexInformer
}

func (r *RoutingPolicyController) Added(ctx context.Context, obj interface{}) error {
	routingPolicy, ok := obj.(*v1.RoutingPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.RoutingPolicy", obj)
	}
	r.RoutingPolicyHandler.Added(ctx, routingPolicy)
	return nil
}

func (r *RoutingPolicyController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	routingPolicy, ok := obj.(*v1.RoutingPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.RoutingPolicy", obj)
	}
	r.RoutingPolicyHandler.Updated(ctx, routingPolicy)
	return nil
}

func (r *RoutingPolicyController) Deleted(ctx context.Context, obj interface{}) error {
	routingPolicy, ok := obj.(*v1.RoutingPolicy)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.RoutingPolicy", obj)
	}
	err := r.RoutingPolicyHandler.Deleted(ctx, routingPolicy)
	return err
}

func (d *RoutingPolicyController) GetProcessItemStatus(obj interface{}) (string, error) {
	return common.NotProcessed, nil
}

func (d *RoutingPolicyController) UpdateProcessItemStatus(obj interface{}, status string) error {
	return nil
}

func NewRoutingPoliciesController(stopCh <-chan struct{}, handler RoutingPolicyHandler, config *rest.Config, resyncPeriod time.Duration, clientLoader loader.ClientLoader) (*RoutingPolicyController, error) {

	rpController := RoutingPolicyController{}
	rpController.RoutingPolicyHandler = handler

	var err error

	rpController.K8sClient, err = clientLoader.LoadKubeClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing policy controller k8s client: %v", err)
	}

	rpController.CrdClient, err = clientLoader.LoadAdmiralClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing policy controller crd client: %v", err)
	}

	rpController.IstioClient, err = versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination rule controller k8s client: %v", err)
	}

	rpController.informer = informerV1.NewRoutingPolicyInformer(
		rpController.CrdClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)

	NewController("rp-ctrl", config.Host, stopCh, &rpController, rpController.informer)
	return &rpController, nil

}

func (t *RoutingPolicyController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	routingPolicy, ok := obj.(*v1.RoutingPolicy)
	if !ok {
		return
	}
	metadata := routingPolicy.ObjectMeta
	if metadata.Annotations[common.AdmiralIgnoreAnnotation] == "true" || metadata.Labels[common.AdmiralIgnoreAnnotation] == "true" {
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.GlobalTrafficPolicyResourceType,
			routingPolicy.Name, routingPolicy.Namespace, "", "Value=true")
	}
}

func (t *RoutingPolicyController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	/*rp, ok := obj.(*v1.RoutingPolicy)
	if ok && t.CrdClient != nil {
		return t.CrdClient.AdmiralV1().RoutingPolicies(rp.Namespace).Get(ctx, rp.Name, meta_v1.GetOptions{})
	}*/
	return nil, fmt.Errorf("crd client is not initialized, txId=%s", ctx.Value("txId"))
}
