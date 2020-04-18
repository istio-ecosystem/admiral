package clusters

import (
	"errors"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"strconv"
	"strings"

	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sV1 "k8s.io/api/core/v1"
)

func GetMeshPorts(clusterName string, destService *k8sV1.Service,
	destDeployment *k8sAppsV1.Deployment) map[string]uint32 {
	var ports = make(map[string]uint32)
	var meshPorts = destDeployment.Spec.Template.Annotations[common.SidecarEnabledPorts]
	if len(meshPorts) == 0 {
		log.Infof(LogFormat, "GetMeshPorts", "service", destService.Name, clusterName, "No mesh ports present, defaulting to first port")
		if destService.Spec.Ports != nil && len(destService.Spec.Ports) > 0 {
			var name = destService.Spec.Ports[0].Name
			if len(name) == 0 {
				name = common.Http
			}
			ports[name] = uint32(destService.Spec.Ports[0].Port)
		}
		return ports
	}

	meshPortsSplit := strings.Split(meshPorts, ",")
	var meshPortMap = make(map[uint32]uint32)
	for _, meshPort := range meshPortsSplit {
		port, err := strconv.ParseUint(meshPort, 10, 32)
		if err != nil {
			continue
		}
		meshPortMap[uint32(port)] = uint32(port)
	}
	for _, servicePort := range destService.Spec.Ports {
		if _, ok := meshPortMap[uint32(servicePort.Port)]; ok {
			log.Debugf(LogFormat, "GetMeshPorts", servicePort.Port, destService.Name, clusterName, "Adding mesh port")
			ports[common.Http] = uint32(servicePort.Port)
		}
	}
	return ports
}

func GetMeshPortsForRollout(clusterName string, destService *k8sV1.Service,
	destRollout *argo.Rollout) map[string]uint32 {
	var ports = make(map[string]uint32)
	var meshPorts = destRollout.Spec.Template.Annotations[common.SidecarEnabledPorts]
	if len(meshPorts) == 0 {
		log.Infof(LogFormat, "GetMeshPorts", "service", destService.Name, clusterName, "No mesh ports present, defaulting to first port")
		if destService.Spec.Ports != nil && len(destService.Spec.Ports) > 0 {
			var name = destService.Spec.Ports[0].Name
			if len(name) == 0 {
				name = common.Http
			}
			ports[name] = uint32(destService.Spec.Ports[0].Port)
		}
		return ports
	}

	meshPortsSplit := strings.Split(meshPorts, ",")
	var meshPortMap = make(map[uint32]uint32)
	for _, meshPort := range meshPortsSplit {
		port, err := strconv.ParseUint(meshPort, 10, 32)
		if err != nil {
			continue
		}
		meshPortMap[uint32(port)] = uint32(port)
	}
	for _, servicePort := range destService.Spec.Ports {
		if _, ok := meshPortMap[uint32(servicePort.Port)]; ok {
			log.Debugf(LogFormat, "GetMeshPorts", servicePort.Port, destService.Name, clusterName, "Adding mesh port")
			ports[common.Http] = uint32(servicePort.Port)
		}
	}
	return ports
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
