package clusters

import (
	"context"
	"errors"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networkingAlpha3 "istio.io/api/networking/v1alpha3"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleEventForOutlierDetection(t *testing.T) {
	ctx := context.Background()

	admiralParamTest := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &common.LabelSet{
			AdmiralCRDIdentityLabel: "assetAlias",
		},
	}

	common.ResetSync()
	registryTest, _ := InitAdmiral(ctx, admiralParamTest)

	type args struct {
		event       admiral.EventType
		od          *v1.OutlierDetection
		clusterName string
		modifySE    ModifySEFunc
	}

	odConfig := model.OutlierConfig{
		BaseEjectionTime:         0,
		ConsecutiveGatewayErrors: 0,
		Interval:                 0,
		XXX_NoUnkeyedLiteral:     struct{}{},
		XXX_unrecognized:         nil,
		XXX_sizecache:            0,
	}

	od := v1.OutlierDetection{
		TypeMeta:   metaV1.TypeMeta{},
		ObjectMeta: metaV1.ObjectMeta{},
		Spec: model.OutlierDetection{
			OutlierConfig:        &odConfig,
			Selector:             map[string]string{"identity": "payments", "env": "e2e"},
			XXX_NoUnkeyedLiteral: struct{}{},
			XXX_unrecognized:     nil,
			XXX_sizecache:        0,
		},
		Status: v1.OutlierDetectionStatus{},
	}

	seFunc := func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {
		return nil, nil
	}

	testArg1 := args{
		event: admiral.Add,
		od: &v1.OutlierDetection{
			Spec:       od.Spec,
			ObjectMeta: metaV1.ObjectMeta{Name: "od1", Namespace: "ns1", Labels: map[string]string{"assetAlias": "Intuit.devx.supercar", "identity": "id", "admiral.io/env": "stage"}},
			TypeMeta: metaV1.TypeMeta{
				Kind:       "admiral.io/v1",
				APIVersion: common.OutlierDetection,
			},
		},
		clusterName: "test",
		modifySE:    seFunc,
	}

	testArg2 := args{
		event: admiral.Add,
		od: &v1.OutlierDetection{
			Spec:       od.Spec,
			ObjectMeta: metaV1.ObjectMeta{Name: "od1", Namespace: "ns1", Labels: map[string]string{"foo": "bar"}},
			TypeMeta: metaV1.TypeMeta{
				Kind:       "admiral.io/v1",
				APIVersion: common.OutlierDetection,
			},
		},
		clusterName: "test",
		modifySE:    seFunc,
	}

	errors.New("foo")

	tests := []struct {
		name   string
		args   args
		expErr error
	}{
		{"identity label missing", testArg2, errors.New("Skipped as label assetAlias was not found")},
		{"happy path", testArg1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HandleEventForOutlierDetection(ctx, tt.args.event, tt.args.od, registryTest, tt.args.clusterName, tt.args.modifySE)
			if tt.expErr != nil {
				assert.Contains(t, err.Error(), tt.expErr.Error())
			} else {
				assert.Nil(t, err, "Not expecting error")
			}
		})
	}
}
