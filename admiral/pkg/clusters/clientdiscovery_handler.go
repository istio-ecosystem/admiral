package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
)

type ClientDiscoveryHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (cdh *ClientDiscoveryHandler) Added(ctx context.Context, obj *admiral.K8sObject) error {
	err := HandleEventForClientDiscovery (ctx, admiral.Add, obj, cdh.RemoteRegistry, cdh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.JobResourceType, obj.Name, cdh.ClusterID, err)
	}
	return err
}

