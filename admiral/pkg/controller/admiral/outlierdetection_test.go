package admiral

import (
	"context"
	"sync"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	"github.com/stretchr/testify/assert"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNewOutlierDetectionController(t *testing.T) {
	//TODO : Test when add update method get implemented

	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	handler := test.MockOutlierDetectionHandler{}

	outlierDetectionController, err := NewOutlierDetectionController(stop, &handler, config, 0, loader.GetFakeClientLoader())

	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	assert.NotNil(t, outlierDetectionController, "OutlierDetectionController is nil")
}

func makeOutlierDetectionTestModel() model.OutlierDetection {
	odConfig := model.OutlierConfig{
		BaseEjectionTime:         0,
		ConsecutiveGatewayErrors: 0,
		Interval:                 0,
		XXX_NoUnkeyedLiteral:     struct{}{},
		XXX_unrecognized:         nil,
		XXX_sizecache:            0,
	}

	od := model.OutlierDetection{
		Selector:      map[string]string{"identity": "payments", "env": "e2e"},
		OutlierConfig: &odConfig,
	}

	return od
}

func TestOutlierDetectionAddUpdateDelete(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})

	odName := "od1"
	od := makeOutlierDetectionTestModel()

	handler := test.MockOutlierDetectionHandler{}

	controller, err := NewOutlierDetectionController(stop, &handler, config, 0, loader.GetFakeClientLoader())
	if err != nil {
		t.Errorf("Unexpected err %v", err)
	}

	assert.NotNil(t, controller, "OutlerDetection controller should not be nil")

	ctx := context.Background()

	addODK8S := makeK8sOutlierDetectionObj(odName, "namespace1", od)
	controller.Added(ctx, addODK8S)

	assert.NotNil(t, handler.Obj, "OutlierHandler object is empty")
	assert.Equal(t, handler.Obj.Spec, addODK8S.Spec, "OutlierDetection spec didn't match")

	updatedOd := model.OutlierDetection{
		//OutlierConfig: &odConfig,
		Selector: map[string]string{"identity": "payments", "env": "qa"},
	}
	updateODK8S := makeK8sOutlierDetectionObj(odName, "namespace1", updatedOd)
	controller.Updated(ctx, updateODK8S, addODK8S)

	assert.NotNil(t, handler.Obj, "OutlierHandler object is empty")
	assert.Equal(t, handler.Obj.Spec, updateODK8S.Spec, "OutlierDetection spec didn't match")

	controller.Deleted(ctx, updateODK8S)
	assert.Nil(t, handler.Obj, "After delete Outlier Detection cache should be empty")

}

func TestOutlierDetectionController_UpdateProcessItemStatus(t *testing.T) {

	odConfig := model.OutlierConfig{
		BaseEjectionTime:         0,
		ConsecutiveGatewayErrors: 0,
		Interval:                 0,
		XXX_NoUnkeyedLiteral:     struct{}{},
		XXX_unrecognized:         nil,
		XXX_sizecache:            0,
	}

	od1 := makeK8sOutlierDetectionObj("od1", "ns1", model.OutlierDetection{
		OutlierConfig:        &odConfig,
		Selector:             map[string]string{"identity": "payments", "env": "e2e"},
		XXX_NoUnkeyedLiteral: struct{}{},
		XXX_unrecognized:     nil,
		XXX_sizecache:        0,
	})

	od2 := makeK8sOutlierDetectionObj("od2", "ns1", model.OutlierDetection{
		OutlierConfig:        &odConfig,
		Selector:             map[string]string{"identity": "payments", "env": "stage"},
		XXX_NoUnkeyedLiteral: struct{}{},
		XXX_unrecognized:     nil,
		XXX_sizecache:        0,
	})

	odCache := &odCache{
		cache: make(map[string]map[string]map[string]*odItems),
		mutex: &sync.RWMutex{},
	}
	odCache.Put(od1)

	type args struct {
		i              interface{}
		status         string
		expectedStatus string
	}

	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})

	handler := test.MockOutlierDetectionHandler{}

	controller, _ := NewOutlierDetectionController(stop, &handler, config, 0, loader.GetFakeClientLoader())

	controller.cache = odCache

	testArgs1 := args{
		i:              od1,
		status:         common.ProcessingInProgress,
		expectedStatus: common.ProcessingInProgress,
	}

	testArgs2 := args{
		i:              od2,
		status:         common.ProcessingInProgress,
		expectedStatus: common.NotProcessed,
	}
	tests := []struct {
		name string
		//fields  fields
		args    args
		wantErr bool
	}{
		{"happypath", testArgs1, false},
		{"cachemiss", testArgs2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := controller.UpdateProcessItemStatus(tt.args.i, tt.args.status)
			if tt.wantErr {
				assert.NotNil(t, err, "expected error for testcase "+tt.name)
				gotStatus, err := controller.GetProcessItemStatus(tt.args.i)
				assert.Nil(t, err, "expected no error while getting status")
				assert.Equal(t, gotStatus, tt.args.expectedStatus)
			} else {
				assert.Nil(t, err, "expected no error for testcase "+tt.name)
				gotStatus, err := controller.GetProcessItemStatus(tt.args.i)
				assert.Nil(t, err, "expected no error while getting status")
				assert.Equal(t, gotStatus, tt.args.expectedStatus)
			}
		})
	}
}

func makeK8sOutlierDetectionObj(name string, namespace string, od model.OutlierDetection) *v1.OutlierDetection {
	return &v1.OutlierDetection{
		Spec:       od,
		ObjectMeta: metaV1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"identity": "id", "admiral.io/env": "stage"}},
		TypeMeta: metaV1.TypeMeta{
			Kind:       "admiral.io/v1",
			APIVersion: common.OutlierDetection,
		},
	}
}

func TestOutlierDetectionController_Get(t *testing.T) {
	type fields struct {
		cache map[string]map[string]map[string]*odItems
		mutex *sync.RWMutex
	}
	type args struct {
		key       string
		namespace string
		addOD     *v1.OutlierDetection
	}

	testFields := fields{
		cache: nil,
		mutex: nil,
	}

	testFields.cache = make(map[string]map[string]map[string]*odItems)
	testFields.mutex = &sync.RWMutex{}

	addOD1 := makeK8sOutlierDetectionObj("foo", "foo", makeOutlierDetectionTestModel())

	testArgs := args{
		key:       "stage.id",
		namespace: "foo",
		addOD:     addOD1,
	}

	var wantOD []*v1.OutlierDetection = make([]*v1.OutlierDetection, 1)

	wantOD[0] = addOD1

	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*v1.OutlierDetection
	}{
		{"Simple GET Test", testFields, testArgs, wantOD},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &odCache{
				cache: tt.fields.cache,
				mutex: tt.fields.mutex,
			}
			c.Put(tt.args.addOD)

			assert.Equalf(t, tt.want, c.Get(tt.args.key, tt.args.namespace), "Get(%v, %v)", tt.args.key, tt.args.namespace)
		})
	}
}

func TestLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not an OutlierDetection object
	o := &OutlierDetectionController{}
	o.LogValueOfAdmiralIoIgnore("not an outlier detection")
	// No error should occur

	// Test case 2: OutlierDetection has no annotations or labels
	o = &OutlierDetectionController{}
	o.LogValueOfAdmiralIoIgnore(&v1.OutlierDetection{})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is not set
	o = &OutlierDetectionController{}
	od := &v1.OutlierDetection{ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{"other-annotation": "value"}}}
	o.LogValueOfAdmiralIoIgnore(od)
	// No error should occur

	// Test case 4: AdmiralIgnoreAnnotation is set in annotations
	o = &OutlierDetectionController{}
	od = &v1.OutlierDetection{ObjectMeta: metaV1.ObjectMeta{Annotations: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	o.LogValueOfAdmiralIoIgnore(od)
	// No error should occur

	// Test case 5: AdmiralIgnoreAnnotation is set in labels
	o = &OutlierDetectionController{}
	od = &v1.OutlierDetection{ObjectMeta: metaV1.ObjectMeta{Labels: map[string]string{common.AdmiralIgnoreAnnotation: "true"}}}
	o.LogValueOfAdmiralIoIgnore(od)
	// No error should occur
}
