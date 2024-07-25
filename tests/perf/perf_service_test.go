package perf

import (
	"fmt"
	"time"

	"github.com/jamiealquiza/tachymeter"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ PerfHandler = (*ServicePerfHandler)(nil)

type ServicePerfHandler struct {
	source      ClusterAssetMap
	destination ClusterAssetMap
}

func NewServicePerfHandler(sourceClusterAssetMap, destinationClusterAssetMap ClusterAssetMap) *tachymeter.Metrics {
	a := &ServicePerfHandler{
		source:      sourceClusterAssetMap,
		destination: destinationClusterAssetMap,
	}

	return a.Run()
}

func (a *ServicePerfHandler) Run() *tachymeter.Metrics {
	defer a.Revert()
	return computeMetrics(a.Action(), a.Reaction())
}

func (a *ServicePerfHandler) Action() TimeMap {
	timeMap := make(TimeMap)

	for destinationAsset, destinationClusters := range a.destination {
		client := getKubeClient(destinationClusters.west)
		namespace := getNamespaceName(destinationAsset)
		dep, err := client.AppsV1().Deployments(namespace).Get(ctx, getDeploymentName(destinationAsset), metav1.GetOptions{})
		if dep != nil && err == nil {
			timeMap[destinationAsset] = handleDeployment(destinationClusters.west, destinationAsset, RegionWest, TempServiceIdentifier)
		} else {
			timeMap[destinationAsset] = handleRollout(destinationClusters.west, destinationAsset, TempServiceIdentifier)
		}
	}

	return timeMap
}

func (a *ServicePerfHandler) Reaction() TimeMultiMap {
	timeMap := make(TimeMultiMap)

	for sourceAsset, sourceClusters := range a.source {
		timeMap[sourceAsset] = make([]time.Time, 0)

		fmt.Printf("\twaiting for service entries to get updated in cluster %q\n", sourceClusters.west)

		for destinationAsset, destinationClusters := range a.destination {
			if sourceClusters.west == destinationClusters.west {
				timeMap[destinationAsset] = append(timeMap[destinationAsset], a.wait(sourceClusters.west, sourceAsset, destinationAsset))
			}
		}
	}

	return timeMap
}

func (a *ServicePerfHandler) Revert() {
	for destinationAsset, destinationClusters := range a.destination {
		client := getKubeClient(destinationClusters.west)
		namespace := getNamespaceName(destinationAsset)
		deploymentName := getDeploymentName(destinationAsset)
		dep, err := client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if dep != nil && err == nil {
			handleDeployment(destinationClusters.west, destinationAsset, TempServiceIdentifier, RegionWest)
		} else {
			handleRollout(destinationClusters.west, destinationAsset, StableServiceIdentifier)
		}
	}
}

func (a *ServicePerfHandler) wait(sourceCluster, sourceAsset, destinationAsset string) time.Time {
	var ts time.Time
	serviceEntryName := getServiceEntryName(destinationAsset)

	Eventually(func(g Gomega) {
		se, err := getIstioClient(sourceCluster).NetworkingV1alpha3().ServiceEntries(SyncNamespace).Get(ctx, serviceEntryName, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(se).ToNot(BeNil())
		g.Expect(se.Spec.ExportTo).To(ContainElement(getNamespaceName(sourceAsset)))
		g.Expect(len(se.Spec.Hosts)).To(Equal(1))
		g.Expect(len(se.Spec.Addresses)).To(Equal(1))
		g.Expect(len(se.Spec.Endpoints)).To(Equal(2))
		localAddress := getLocalServiceEntryAddress(getServiceName(destinationAsset, TempServiceIdentifier), getNamespaceName(destinationAsset))
		g.Expect(se.Spec.Endpoints).To(ContainElement(HaveField("Address", Equal(localAddress))))
		ts = getLastUpdatedTime(se.GetAnnotations())
	}).WithTimeout(ServiceEntryWaitTime).WithPolling(time.Second).Should(Succeed())

	return ts
}

func handleDeployment(cluster, asset, oldServiceIdentifier, newServiceIdentifier string) time.Time {
	namespace := getNamespaceName(asset)
	client := getKubeClient(cluster)
	Expect(client.CoreV1().Services(namespace).Delete(ctx, getServiceName(asset, oldServiceIdentifier), metav1.DeleteOptions{})).ToNot(HaveOccurred())

	svc, err := client.CoreV1().Services(namespace).Create(ctx, getServiceSpec(asset, newServiceIdentifier), metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())
	Expect(svc).ToNot(BeNil())

	return getLastUpdatedTime(svc.GetAnnotations())
}

func handleRollout(cluster, asset, serviceIdentifier string) time.Time {
	kubeClient := getKubeClient(cluster)
	argoClient := getArgoClient(cluster)
	namespace := getNamespaceName(asset)

	if serviceIdentifier == TempServiceIdentifier {
		kubeClient.CoreV1().Services(namespace).Create(ctx, getServiceSpec(asset, TempServiceIdentifier), metav1.CreateOptions{})
	}

	ro, err := argoClient.ArgoprojV1alpha1().Rollouts(namespace).Get(ctx, getRolloutName(asset), metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	Expect(ro).ToNot(BeNil())

	ro.Spec.Strategy.Canary.StableService = getServiceName(asset, serviceIdentifier)

	ro.Generation++

	ro, err = argoClient.ArgoprojV1alpha1().Rollouts(namespace).Update(ctx, ro, metav1.UpdateOptions{})
	Expect(err).ToNot(HaveOccurred())
	Expect(ro).ToNot(BeNil())

	return getLastUpdatedTime(ro.GetAnnotations())
}
