package clusters

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)

type JobHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (jh *JobHandler) Added(ctx context.Context, obj *admiral.K8sObject) error {
	err := HandleEventForClientDiscovery (ctx, admiral.Add, obj, jh.RemoteRegistry, jh.ClusterID)
	if err != nil {
		return fmt.Errorf(LogErrFormat, common.Add, common.JobResourceType, obj.Name, jh.ClusterID, err)
	}
	return err
}

func (jh *JobHandler) Updated(ctx context.Context, obj *admiral.K8sObject) error {
	log.Infof(LogFormat, common.Update, common.JobResourceType, obj.Name, jh.ClusterID, common.ReceivedStatus)
	return nil
}

func (jh *JobHandler) Deleted(ctx context.Context, obj *admiral.K8sObject) error {
	//not implemented
	return nil
}

