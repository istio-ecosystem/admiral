package clusters

import (
	"context"

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
)

// SidecarHandler responsible for handling Add/Update/Delete events for
// Sidecar resources
type SidecarHandler struct {
	RemoteRegistry *RemoteRegistry
	ClusterID      string
}

func (dh *SidecarHandler) Added(ctx context.Context, obj *v1alpha3.Sidecar) error {
	return nil
}

func (dh *SidecarHandler) Updated(ctx context.Context, obj *v1alpha3.Sidecar) error {
	return nil
}

func (dh *SidecarHandler) Deleted(ctx context.Context, obj *v1alpha3.Sidecar) error {
	return nil
}
