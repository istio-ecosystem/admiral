package clusters

import (
	"errors"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

func GetMeshPorts(clusterName string, destService *k8sV1.Service,
	destDeployment *k8sAppsV1.Deployment) map[string]uint32 {
	var meshPorts = destDeployment.Spec.Template.Annotations[common.SidecarEnabledPorts]
	ports := getMeshPortsHelper(meshPorts, destService, clusterName)
	return ports
}

func GetMeshPortsForRollout(clusterName string, destService *k8sV1.Service,
	destRollout *argo.Rollout) map[string]uint32 {
	var meshPorts = destRollout.Spec.Template.Annotations[common.SidecarEnabledPorts]
	ports := getMeshPortsHelper(meshPorts, destService, clusterName)
	return ports
}

func getMeshPortsHelper(meshPorts string, destService *k8sV1.Service, clusterName string) map[string]uint32 {
	var ports = make(map[string]uint32)

	if destService == nil {
		return ports
	}
	if len(meshPorts) == 0 {
		log.Infof(LogFormat, "GetMeshPorts", "service", destService.Name, clusterName, "No mesh ports present, defaulting to first port")
		if destService.Spec.Ports != nil && len(destService.Spec.Ports) > 0 {
			var protocol = GetPortProtocol(destService.Spec.Ports[0].Name)
			ports[protocol] = uint32(destService.Spec.Ports[0].Port)
		}
		return ports
	}

	meshPortsSplit := strings.Split(meshPorts, ",")

	if len(meshPortsSplit) > 1 {
		log.Warnf(LogErrFormat, "Get", "MeshPorts", "", clusterName,
			"Multiple inbound mesh ports detected, admiral generates service entry with first matched port and protocol")
	}

	//fetch the first valid port if there is more than one mesh port
	var meshPortMap = make(map[uint32]uint32)
	for _, meshPort := range meshPortsSplit {
		port, err := strconv.ParseUint(meshPort, 10, 32)
		if err == nil {
			meshPortMap[uint32(port)] = uint32(port)
			break
		}
	}
	for _, servicePort := range destService.Spec.Ports {
		//handling relevant protocols from here:
		// https://istio.io/latest/docs/ops/configuration/traffic-management/protocol-selection/#manual-protocol-selection
		//use target port if present to match the annotated mesh port
		targetPort := uint32(servicePort.Port)
		if servicePort.TargetPort.StrVal != "" {
			port, err := strconv.Atoi(servicePort.TargetPort.StrVal)
			if err != nil {
				log.Warnf(LogErrFormat, "GetMeshPorts", "Failed to parse TargetPort", destService.Name, clusterName, err)
			}
			if port > 0 {
				targetPort = uint32(port)
			}

		}
		if servicePort.TargetPort.IntVal != 0 {
			targetPort = uint32(servicePort.TargetPort.IntVal)
		}
		if _, ok := meshPortMap[targetPort]; ok {
			var protocol = GetPortProtocol(servicePort.Name)
			log.Debugf(LogFormat, "GetMeshPorts", servicePort.Port, destService.Name, clusterName, "Adding mesh port for protocol: " + protocol)
			ports[protocol] = uint32(servicePort.Port)
			break
		}
	}
	return ports
}

func GetPortProtocol(name string) string {
	var protocol = common.Http
	if strings.Index(name, common.GrpcWeb) == 0 {
		protocol = common.GrpcWeb
	} else if strings.Index(name, common.Grpc) == 0 {
		protocol = common.Grpc
	} else if strings.Index(name, common.Http2) == 0 {
		protocol = common.Http2
	}
	return protocol
}

func GetServiceEntryStateFromConfigmap(configmap *k8sV1.ConfigMap) *ServiceEntryAddressStore {

	bytes := []byte(configmap.Data["serviceEntryAddressStore"])
	addressStore := ServiceEntryAddressStore{}
	err := yaml.Unmarshal(bytes, &addressStore)

	if err != nil {
		log.Errorf("Could not unmarshal configmap data. Double check the configmap format. %v", err)
		return nil
	}
	if addressStore.Addresses == nil {
		addressStore.Addresses = []string{}
	}
	if addressStore.EntryAddresses == nil {
		addressStore.EntryAddresses = map[string]string{}
	}

	return &addressStore
}

func ValidateConfigmapBeforePutting(cm *k8sV1.ConfigMap) error {
	if cm.ResourceVersion == "" {
		return errors.New("resourceversion required") //without it, we can't be sure someone else didn't put something between our read and write
	}
	store := GetServiceEntryStateFromConfigmap(cm)
	if len(store.EntryAddresses) != len(store.Addresses) {
		return errors.New("address cache length mismatch") //should be impossible. We're in a state where the list of addresses doesn't match the map of se:address. Something's been missed and must be fixed
	}
	return nil
}
