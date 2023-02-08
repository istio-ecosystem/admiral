package clusters

import (
	"fmt"
	"strings"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
)

// DependencyProxyConverter provides functions that help convert
// the given configuration to the corresponding VirtualService
type DependencyProxyConverter interface {
	VirtualServiceHostsGenerator
	VirtualServiceDestinationHostGenerator
}

// VirtualServiceDestinationHostGenerator generates the hostname of the proxy
// mesh service
type VirtualServiceDestinationHostGenerator interface {
	GenerateProxyDestinationHostName(dependencyProxyObj *v1.DependencyProxy) (string, error)
}

// VirtualServiceHostsGenerator generates all the host names
type VirtualServiceHostsGenerator interface {
	GenerateVirtualServiceHostNames(dependencyProxyObj *v1.DependencyProxy) ([]string, error)
}

type dependencyProxyConverter struct {
	*virtualServiceHostNameGenerator
	*virtualServiceDestinationHostHostGenerator
}

type virtualServiceHostNameGenerator struct {
}
type virtualServiceDestinationHostHostGenerator struct {
}

// GenerateProxyDestinationHostName generates the VirtualServices's destination host which in this case
// would be the endpoint of the proxy identity
func (*virtualServiceDestinationHostHostGenerator) GenerateProxyDestinationHostName(dependencyProxyObj *v1.DependencyProxy) (string, error) {
	err := validate(dependencyProxyObj)
	if err != nil {
		return "", fmt.Errorf("failed to generate virtual service destination hostname due to error: %w", err)
	}
	proxyEnv := dependencyProxyObj.ObjectMeta.Annotations[common.GetEnvKey()]
	proxyIdentity := dependencyProxyObj.Spec.Proxy.Identity
	return strings.ToLower(common.GetCnameVal([]string{proxyEnv, proxyIdentity, common.GetHostnameSuffix()})), nil
}

// GenerateVirtualServiceHostNames generates all the VirtualService's hostnames using the informartion in the
//  *v1.DependencyProxy object. In addition it also generates a default hostname by concatenating the
// admiral.io/env value with destinationServiceIdentity+dnsSuffix
func (*virtualServiceHostNameGenerator) GenerateVirtualServiceHostNames(dependencyProxyObj *v1.DependencyProxy) ([]string, error) {
	err := validate(dependencyProxyObj)
	if err != nil {
		return nil, fmt.Errorf("failed to generate virtual service hostnames due to error: %w", err)
	}
	destinationServiceIdentity := dependencyProxyObj.Spec.Destination.Identity
	dnsSuffix := dependencyProxyObj.Spec.Destination.DnsSuffix
	proxyEnv := dependencyProxyObj.ObjectMeta.Annotations[common.GetEnvKey()]

	defaultVSHostName := strings.ToLower(common.GetCnameVal([]string{proxyEnv, destinationServiceIdentity, dnsSuffix}))

	vsHostNames := make([]string, 0)
	vsHostNames = append(vsHostNames, defaultVSHostName)
	if dependencyProxyObj.Spec.Destination.DnsPrefixes == nil {
		return vsHostNames, nil
	}
	dnsPrefixes := dependencyProxyObj.Spec.Destination.DnsPrefixes
	for _, prefix := range dnsPrefixes {
		vsHostNames = append(vsHostNames, common.GetCnameVal([]string{prefix, destinationServiceIdentity, dnsSuffix}))
	}
	return vsHostNames, nil
}

func validate(dependencyProxyObj *v1.DependencyProxy) error {

	if dependencyProxyObj == nil {
		return fmt.Errorf("dependencyProxyObj is nil")
	}
	if dependencyProxyObj.ObjectMeta.Annotations == nil {
		return fmt.Errorf("dependencyProxyObj.ObjectMeta.Annotations is nil")
	}
	proxyEnv := dependencyProxyObj.ObjectMeta.Annotations[common.GetEnvKey()]
	if proxyEnv == "" {
		return fmt.Errorf("%s is empty", common.GetEnvKey())
	}
	if dependencyProxyObj.Spec.Proxy == nil {
		return fmt.Errorf("dependencyProxyObj.Spec.Proxy is nil")
	}
	proxyIdentity := dependencyProxyObj.Spec.Proxy.Identity
	if proxyIdentity == "" {
		return fmt.Errorf("dependencyProxyObj.Spec.Proxy.Identity is empty")
	}

	if dependencyProxyObj.Spec.Destination == nil {
		return fmt.Errorf("dependencyProxyObj.Spec.Destination is nil")
	}
	destinationServiceIdentity := dependencyProxyObj.Spec.Destination.Identity
	if destinationServiceIdentity == "" {
		return fmt.Errorf("dependencyProxyObj.Spec.Destination.Identity is empty")
	}
	dnsSuffix := dependencyProxyObj.Spec.Destination.DnsSuffix
	if dnsSuffix == "" {
		return fmt.Errorf("dependencyProxyObj.Spec.Destination.DnsSuffix is empty")
	}

	return nil
}
