package clusters

import (
	"github.com/admiral/admiral/pkg/controller/common"
	"istio.io/istio/pkg/log"
	"strconv"
	"strings"

	k8sV1 "k8s.io/api/core/v1"
	k8sAppsV1 "k8s.io/api/apps/v1"
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
