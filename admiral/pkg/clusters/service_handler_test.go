package clusters

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	argoFake "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/fake"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	apiMachineryMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sFake "k8s.io/client-go/kubernetes/fake"
)

func TestHandleEventForService(t *testing.T) {
	ctx := context.Background()
	params := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}

	registry, _ := InitAdmiral(context.Background(), params)

	deploymentController := &admiral.DeploymentController{K8sClient: k8sFake.NewSimpleClientset(&appsV1.Deployment{})}
	remoteController1, _ := createMockRemoteController(func(i interface{}) {})
	remoteController1.DeploymentController = deploymentController
	remoteController1.RolloutController = nil

	rolloutController := &admiral.RolloutController{RolloutClient: argoFake.NewSimpleClientset(&argo.Rollout{}).ArgoprojV1alpha1()}
	remoteController2, _ := createMockRemoteController(func(i interface{}) {})
	remoteController2.RolloutController = rolloutController
	remoteController2.DeploymentController = nil

	registry.remoteControllers = map[string]*RemoteController{"cluster-1": remoteController1, "cluster-2": remoteController2}

	serviceWithSelector := &coreV1.Service{
		ObjectMeta: apiMachineryMetaV1.ObjectMeta{
			Name:        "testservice",
			Annotations: map[string]string{"admiral.io/env": "testenv"},
			Namespace:   "namespace",
		},
	}
	serviceWithSelector.Spec.Selector = map[string]string{"app": "debug"}

	serviceWithoutSelector := &coreV1.Service{
		ObjectMeta: apiMachineryMetaV1.ObjectMeta{
			Name:        "testservice",
			Annotations: map[string]string{"admiral.io/env": "testenv"},
			Namespace:   "namespace",
		},
	}
	testcases := []struct {
		name               string
		service            *coreV1.Service
		remoteRegistry     *RemoteRegistry
		clusterName        string
		ArgoRolloutEnabled bool
		error              error
	}{
		{
			name:               "if selector of the Service is nil",
			service:            serviceWithoutSelector,
			remoteRegistry:     registry,
			clusterName:        "cluster1",
			ArgoRolloutEnabled: false,
			error:              fmt.Errorf("selector missing on service=%s in namespace=%s cluster=%s", "testservice", "namespace", "cluster1"),
		},
		{
			name:               "if remote controller for the cluster does not exist",
			service:            serviceWithSelector,
			remoteRegistry:     registry,
			clusterName:        "clusterX",
			ArgoRolloutEnabled: false,
			error:              fmt.Errorf("could not find the remote controller for cluster=%s", "clusterX"),
		},
		{
			name:               "if deployment controller is not nil",
			service:            serviceWithSelector,
			remoteRegistry:     registry,
			clusterName:        "cluster-1",
			ArgoRolloutEnabled: false,
			error:              nil,
		},
		{
			name:               "if rollout controller is not nil",
			service:            serviceWithSelector,
			remoteRegistry:     registry,
			clusterName:        "cluster-2",
			ArgoRolloutEnabled: true,
			error:              nil,
		},
	}

	for _, c := range testcases {
		t.Run(c.name, func(t *testing.T) {
			common.SetArgoRolloutsEnabled(c.ArgoRolloutEnabled)
			ctx = context.WithValue(ctx, "eventType", admiral.Update)
			err := handleEventForService(ctx, c.service, c.remoteRegistry, c.clusterName)
			if err != nil || c.error != nil {
				assert.Equal(t, err.Error(), c.error.Error())
			} else {
				assert.Equal(t, err, c.error)
			}
		})
	}
}

func TestHandleServiceEventForDeployment(t *testing.T) {
	var (
		deploymentController           *admiral.DeploymentController
		ctx                            = context.TODO()
		labels                         = map[string]string{"app": "app"}
		clusterName                    = "cluster-name"
		deploymentName1                = "deployment1"
		deploymentName2                = "deployment2"
		serviceInNamespace1            = "service1"
		namespace1                     = "namespace1"
		namespace2                     = "namespace2"
		deployment1InNamespace1        = newFakeDeployment(deploymentName1, namespace1, labels)
		deployment2InNamespace1        = newFakeDeployment(deploymentName2, namespace1, labels)
		deployment1InNamespace2        = newFakeDeployment(deploymentName1, namespace2, labels)
		istioIngressGatewayService     = newFakeService(common.IstioIngressGatewayServiceName, common.NamespaceIstioSystem, labels)
		applicationServiceInNamespace1 = newFakeService(serviceInNamespace1, namespace1, labels)

		remoteControllers = map[string]*RemoteController{
			clusterName: &RemoteController{
				DeploymentController: &admiral.DeploymentController{
					K8sClient: k8sFake.NewSimpleClientset(
						deployment1InNamespace1,
						deployment2InNamespace1,
						deployment1InNamespace2),
					Cache: admiral.NewDeploymentCache(),
				},
			},
		}
		remoteRegistry = newRemoteRegistry(ctx, remoteControllers)
		stop           = make(chan struct{})
		config         = rest.Config{Host: "localhost"}
		resyncPeriod   = time.Millisecond * 1
	)
	deploymentController = remoteControllers[clusterName].DeploymentController
	deploymentController.Cache.UpdateDeploymentToClusterCache("asset1", deployment1InNamespace1)
	deploymentController.Cache.UpdateDeploymentToClusterCache("asset2", deployment2InNamespace1)
	deploymentController.Cache.UpdateDeploymentToClusterCache("asset3", deployment1InNamespace2)

	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	remoteControllers[clusterName].ServiceController = serviceController

	cases := []struct {
		name                         string
		svc                          *coreV1.Service
		fakeHandleEventForDeployment *fakeHandleEventForDeployment
		assertFunc                   func(fakeHandler *fakeHandleEventForDeployment) error
		expectedErr                  error
	}{
		{
			name: "Given, there is a change in a service, and there are two deployments in the same namespace, " +
				"When, HandleServiceEventForDeployment is invoked, " +
				"Then, handler should be called for both the deployments",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForDeployment: newFakeHandleEventForDeploymentsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						deploymentName1: nil,
						deploymentName2: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForDeployment) error {
				if fakeHandler.CalledDeploymentForNamespace(deploymentName1, namespace1) &&
					fakeHandler.CalledDeploymentForNamespace(deploymentName2, namespace1) {
					return nil
				}
				return fmt.Errorf("expected to call both %s and %s",
					deploymentName1, deploymentName2)
			},
			expectedErr: nil,
		},
		{
			name: "Given, there is a change in a service, and there are two deployments in the same namespace, " +
				"When, HandleServiceEventForDeployment is invoked, " +
				"When, handler for deployment returns nil for both deployments, " +
				"Then, it should return nil",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForDeployment: newFakeHandleEventForDeploymentsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						deploymentName1: nil,
						deploymentName2: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForDeployment) error {
				return nil
			},
			expectedErr: nil,
		},
		{
			name: "Given, there is a change in a service, and there are two deployments in the same namespace, " +
				"When, HandleServiceEventForDeployment is invoked, " +
				"When, handler for deployment returns an error for one of the deployments, " +
				"Then, it should process both the deployments, but still return an error",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForDeployment: newFakeHandleEventForDeploymentsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						deploymentName1: nil,
						deploymentName2: fmt.Errorf("error processing %s", deploymentName2),
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForDeployment) error {
				return nil
			},
			expectedErr: fmt.Errorf("error processing %s", deploymentName2),
		},
		{
			name: "Given, there is a change in istio ingressgateway service, " +
				"When, HandleServiceEventForDeployment is invoked, " +
				"Then, it should call handler for deployment with all the deployments in the cluster",
			svc: istioIngressGatewayService,
			fakeHandleEventForDeployment: newFakeHandleEventForDeploymentsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						deploymentName1: nil,
						deploymentName2: nil,
					},
					namespace2: map[string]error{
						deploymentName1: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForDeployment) error {
				if fakeHandler.CalledDeploymentForNamespace(deploymentName1, namespace1) &&
					fakeHandler.CalledDeploymentForNamespace(deploymentName2, namespace1) &&
					fakeHandler.CalledDeploymentForNamespace(deploymentName1, namespace2) {
					return nil
				}
				return nil
			},
		},
		{
			name: "Given, there is a change in a service other than the istio ingressgateway service, " +
				"When, HandleServiceEventForDeployment is invoked, " +
				"Then, it should call handler for deployment with all the deployments in the namespace, " +
				"And, it should not call handler for deployment in namespaces other than the namespace of the service",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForDeployment: newFakeHandleEventForDeploymentsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						deploymentName1: nil,
						deploymentName2: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForDeployment) error {
				if fakeHandler.CalledDeploymentForNamespace(deploymentName1, namespace1) &&
					fakeHandler.CalledDeploymentForNamespace(deploymentName2, namespace1) {
					if fakeHandler.CalledDeploymentForNamespace(deploymentName1, namespace2) {
						return fmt.Errorf(
							"deployment handler called for deployment in %s "+
								"namespace which is not the same as the service namespace, which is: %s",
							namespace2, namespace1)
					}
					return nil
				}
				return fmt.Errorf("deployment handler not called for deployments %s and %s in namespace %s",
					deploymentName1, deploymentName2, namespace1)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx = context.WithValue(ctx, "eventType", admiral.Update)
			err := handleServiceEventForDeployment(
				ctx,
				c.svc,
				remoteRegistry,
				clusterName,
				deploymentController,
				serviceController,
				c.fakeHandleEventForDeployment.handleEventForDeploymentFunc())
			if err != nil && c.expectedErr == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedErr != nil {
				if !(err.Error() == c.expectedErr.Error()) {
					t.Errorf("error mismatch, expected '%v' but got '%v'", c.expectedErr, err)
				}
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error %v but got nil", c.expectedErr)
			}
			err = c.assertFunc(c.fakeHandleEventForDeployment)
			if err != nil {
				t.Errorf("expected assertion to return nil, but got: %v", err)
			}
		})
	}
}

func TestHandleServiceEventForRollout(t *testing.T) {
	var (
		rolloutController              *admiral.RolloutController
		ctx                            = context.TODO()
		labels                         = map[string]string{"app": "app"}
		clusterName                    = "cluster-name"
		rolloutName1                   = "rollout1"
		rolloutName2                   = "rollout2"
		serviceInNamespace1            = "service1"
		namespace1                     = "namespace1"
		namespace2                     = "namespace2"
		rollout1InNamespace1           = newFakeRollout(rolloutName1, namespace1, labels)
		rollout2InNamespace1           = newFakeRollout(rolloutName2, namespace1, labels)
		rollout1InNamespace2           = newFakeRollout(rolloutName1, namespace2, labels)
		istioIngressGatewayService     = newFakeService(common.IstioIngressGatewayServiceName, common.NamespaceIstioSystem, labels)
		applicationServiceInNamespace1 = newFakeService(serviceInNamespace1, namespace1, labels)
		remoteControllers              = map[string]*RemoteController{
			clusterName: &RemoteController{
				RolloutController: &admiral.RolloutController{
					RolloutClient: argoFake.NewSimpleClientset(
						rollout1InNamespace1,
						rollout2InNamespace1,
						rollout1InNamespace2).ArgoprojV1alpha1(),
					Cache: admiral.NewRolloutCache(),
				},
			},
		}
		remoteRegistry = newRemoteRegistry(ctx, remoteControllers)
		stop           = make(chan struct{})
		config         = rest.Config{Host: "localhost"}
		resyncPeriod   = time.Millisecond * 1
	)
	rolloutController = remoteControllers[clusterName].RolloutController
	rolloutController.Cache.UpdateRolloutToClusterCache("asset1", rollout1InNamespace1)
	rolloutController.Cache.UpdateRolloutToClusterCache("asset2", rollout2InNamespace1)
	rolloutController.Cache.UpdateRolloutToClusterCache("asset3", rollout1InNamespace2)

	serviceController, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	remoteControllers[clusterName].ServiceController = serviceController

	cases := []struct {
		name                      string
		svc                       *coreV1.Service
		fakeHandleEventForRollout *fakeHandleEventForRollout
		assertFunc                func(fakeHandler *fakeHandleEventForRollout) error
		expectedErr               error
	}{
		{
			name: "Given, there is a change in a service, and there are two rollouts in the same namespace, " +
				"When, HandleServiceEventForRollout is invoked, " +
				"Then, handler should be called for both the rollout",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						rolloutName1: nil,
						rolloutName2: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForRollout) error {
				if fakeHandler.CalledRolloutForNamespace(rolloutName1, namespace1) &&
					fakeHandler.CalledRolloutForNamespace(rolloutName2, namespace1) {
					return nil
				}
				return fmt.Errorf("expected to call both %s and %s",
					rolloutName1, rolloutName2)
			},
			expectedErr: nil,
		},
		{
			name: "Given, there is a change in a service, and there are two rollouts in the same namespace, " +
				"When, HandleServiceEventForRollout is invoked, " +
				"When, handler for rollout returns nil for both deployments, " +
				"Then, it should return nil",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						rolloutName1: nil,
						rolloutName2: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForRollout) error {
				return nil
			},
			expectedErr: nil,
		},
		{
			name: "Given, there is a change in a service, and there are two rollouts in the same namespace, " +
				"When, HandleServiceEventForRollout is invoked, " +
				"When, handler for rollout returns an error for one of the rollouts, " +
				"Then, it should process both the rollouts, but still return an error",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						rolloutName1: nil,
						rolloutName2: fmt.Errorf("error processing %s", rolloutName2),
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForRollout) error {
				return nil
			},
			expectedErr: fmt.Errorf("error processing %s", rolloutName2),
		},
		{
			name: "Given, there is a change in istio ingressgateway service, " +
				"When, HandleServiceEventForRollout is invoked, " +
				"Then, it should call handler for rollout with all the rollouts in the cluster",
			svc: istioIngressGatewayService,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						rolloutName1: nil,
						rolloutName2: nil,
					},
					namespace2: map[string]error{
						rolloutName1: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForRollout) error {
				if fakeHandler.CalledRolloutForNamespace(rolloutName1, namespace1) &&
					fakeHandler.CalledRolloutForNamespace(rolloutName2, namespace1) &&
					fakeHandler.CalledRolloutForNamespace(rolloutName1, namespace2) {
					return nil
				}
				return nil
			},
		},
		{
			name: "Given, there is a change in a service other than the istio ingressgateway service, " +
				"When, HandleServiceEventForRollout is invoked, " +
				"Then, it should call handler for rollout with all the rollouts in the namespace, " +
				"And, it should not call handler for rollout in namespaces other than the namespace of the service",
			svc: applicationServiceInNamespace1,
			fakeHandleEventForRollout: newFakeHandleEventForRolloutsByError(
				map[string]map[string]error{
					namespace1: map[string]error{
						rolloutName1: nil,
						rolloutName2: nil,
					},
				},
			),
			assertFunc: func(fakeHandler *fakeHandleEventForRollout) error {
				if fakeHandler.CalledRolloutForNamespace(rolloutName1, namespace1) &&
					fakeHandler.CalledRolloutForNamespace(rolloutName2, namespace1) {
					if fakeHandler.CalledRolloutForNamespace(rolloutName1, namespace2) {
						return fmt.Errorf(
							"rollout handler called for deployment in %s "+
								"namespace which is not the same as the service namespace, which is: %s",
							namespace2, namespace1)
					}
					return nil
				}
				return fmt.Errorf("rollout handler not called for rollouts %s and %s in namespace %s",
					rolloutName1, rolloutName2, namespace1)
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx = context.WithValue(ctx, "eventType", admiral.Update)
			err := handleServiceEventForRollout(
				ctx,
				c.svc,
				remoteRegistry,
				clusterName,
				rolloutController,
				serviceController,
				c.fakeHandleEventForRollout.handleEventForRolloutFunc())
			if err != nil && c.expectedErr == nil {
				t.Errorf("expected error to be nil but got %v", err)
			}
			if err != nil && c.expectedErr != nil {
				if !(err.Error() == c.expectedErr.Error()) {
					t.Errorf("error mismatch, expected '%v' but got '%v'", c.expectedErr, err)
				}
			}
			if err == nil && c.expectedErr != nil {
				t.Errorf("expected error %v but got nil", c.expectedErr)
			}
			err = c.assertFunc(c.fakeHandleEventForRollout)
			if err != nil {
				t.Errorf("expected assertion to return nil, but got: %v", err)
			}
		})
	}
}

func newFakeService(name, namespace string, selectorLabels map[string]string) *coreV1.Service {
	return &coreV1.Service{
		ObjectMeta: apiMachineryMetaV1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: coreV1.ServiceSpec{
			Selector: selectorLabels,
		},
	}
}

func TestCheckIfThereAreMultipleMatchingServices(t *testing.T) {
	var (
		labels              = map[string]string{"app": "app"}
		serviceInNamespace1 = "service1"
		serviceInNamespace2 = "service2"
		serviceInNamespace3 = "service3"
		namespace           = "namespace"
		applicationService1 = newFakeService(serviceInNamespace1, namespace, labels)
		applicationService2 = newFakeService(serviceInNamespace2, namespace, labels)
		applicationService3 = newFakeService(serviceInNamespace3, namespace, labels)
		stop                = make(chan struct{})
		config              = rest.Config{Host: "localhost"}
		resyncPeriod        = time.Millisecond * 1
		clusterName         = "test-cluster"
	)

	applicationService1.Spec.Ports = []coreV1.ServicePort{{Name: "http", Protocol: "http", Port: int32(8090), TargetPort: intstr.FromInt(8090)}}
	applicationService2.Spec.Ports = []coreV1.ServicePort{{Name: "http", Protocol: "http", Port: int32(8090), TargetPort: intstr.FromInt(8090)}}

	serviceControllerWithNoService, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}

	serviceControllerWithOneMatchingService, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	serviceControllerWithOneMatchingService.Cache.Put(applicationService1)
	serviceControllerWithOneMatchingService.Cache.Put(applicationService3)

	serviceControllerWithMultipleMatchingService, err := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, resyncPeriod, loader.GetFakeClientLoader())
	if err != nil {
		t.Fatalf("%v", err)
	}
	serviceControllerWithMultipleMatchingService.Cache.Put(applicationService1)
	serviceControllerWithMultipleMatchingService.Cache.Put(applicationService2)

	deployment := appsV1.Deployment{}
	deployment.Name = "depWithSelector"
	deployment.Namespace = "namespace"
	deployment.Spec.Selector = &apiMachineryMetaV1.LabelSelector{MatchLabels: map[string]string{"app": "app"}}
	deployment.Spec.Template.Annotations = map[string]string{common.SidecarEnabledPorts: "8090"}

	rollout := argo.Rollout{}
	rollout.Name = "rolloutWithSelector"
	rollout.Namespace = "namespace"
	rollout.Spec.Selector = &apiMachineryMetaV1.LabelSelector{MatchLabels: map[string]string{"app": "app"}}
	rollout.Spec.Template.Annotations = map[string]string{common.SidecarEnabledPorts: "8090"}

	testCases := []struct {
		name              string
		eventForService   *coreV1.Service
		serviceController *admiral.ServiceController
		obj               interface{}
		expectedRes       bool
		expectedErr       error
	}{
		{
			name: "Given we receive an event for service," +
				"And there are multiple SVC associated to the deployment," +
				"Then we expect to return true",
			serviceController: serviceControllerWithMultipleMatchingService,
			eventForService:   applicationService1,
			obj:               deployment,
			expectedRes:       true,
			expectedErr:       nil,
		},
		{
			name: "Given we receive an event for service," +
				"And there is only one SVC associated to the deployment," +
				"Then we expect to return false",
			serviceController: serviceControllerWithOneMatchingService,
			eventForService:   applicationService1,
			obj:               deployment,
			expectedRes:       false,
			expectedErr:       nil,
		},
		{
			name: "Given we receive an event for service," +
				"And there are multiple SVC associated to the rollout," +
				"Then we expect to return true",
			serviceController: serviceControllerWithMultipleMatchingService,
			eventForService:   applicationService1,
			obj:               rollout,
			expectedRes:       true,
			expectedErr:       nil,
		},
		{
			name: "Given we receive an event for service," +
				"And there is only one SVC associated to the rollout," +
				"Then we expect to return false",
			serviceController: serviceControllerWithOneMatchingService,
			eventForService:   applicationService1,
			obj:               rollout,
			expectedRes:       false,
			expectedErr:       nil,
		},
		{
			name: "Given we receive an event for service," +
				"And there is are multiple SVC associated to the rollout," +
				"And the one we receive the event for does not have mesh ports," +
				"Then we expect to return true",
			serviceController: serviceControllerWithOneMatchingService,
			eventForService:   applicationService3,
			obj:               rollout,
			expectedRes:       true,
			expectedErr:       nil,
		},
		{
			name: "Given we receive an event for service," +
				"And there is only no SVC in the cache for that namespace," +
				"Then we expect to return false," +
				"And the error - service to be deleted does not exist in the cache",
			serviceController: serviceControllerWithNoService,
			eventForService:   applicationService1,
			obj:               rollout,
			expectedRes:       false,
			expectedErr:       fmt.Errorf("service to be deleted does not exist in the cache"),
		},
		{
			name: "Given we receive an event for service," +
				"And the type of the object is not rollout or deployment," +
				"Then we expect to return false," +
				"And the error - type assertion failed, obj is not of type *v1.Deployment or *argo.Rollout",
			serviceController: serviceControllerWithOneMatchingService,
			eventForService:   applicationService1,
			obj:               "notDeploymentOrRollout",
			expectedRes:       false,
			expectedErr:       fmt.Errorf("type assertion failed, notDeploymentOrRollout is not of type *v1.Deployment or *argo.Rollout"),
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			eventType, err := checkIfThereAreMultipleMatchingServices(c.eventForService, c.serviceController, c.obj, clusterName)
			assert.Equal(t, c.expectedRes, eventType)
			assert.Equal(t, c.expectedErr, err)
		})
	}
}
