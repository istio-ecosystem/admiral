package clusters

import (
	"context"
	"github.com/stretchr/testify/assert"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"testing"
)

func TestSidecarHandler_Added(t *testing.T) {
	var dh SidecarHandler
	var ctx context.Context
	var obj *v1alpha3.Sidecar
	err := dh.Added(ctx, obj)
	assert.Nil(t, err)
}

func TestSidecarHandler_Updated(t *testing.T) {
	var dh SidecarHandler
	var ctx context.Context
	var obj *v1alpha3.Sidecar
	err := dh.Updated(ctx, obj)
	assert.Nil(t, err)
}

func TestSidecarHandler_Deleted(t *testing.T) {
	var dh SidecarHandler
	var ctx context.Context
	var obj *v1alpha3.Sidecar
	err := dh.Deleted(ctx, obj)
	assert.Nil(t, err)
}
