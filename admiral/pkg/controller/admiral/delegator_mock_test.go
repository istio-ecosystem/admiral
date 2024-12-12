package admiral

import (
	"context"

	log "github.com/sirupsen/logrus"
)

type MockDelegator struct {
	obj    interface{}
	getErr error
}

func (m *MockDelegator) DoesGenerationMatch(ctx *log.Entry, i interface{}, i2 interface{}) (bool, error) {
	return false, nil
}

func (m *MockDelegator) IsOnlyReplicaCountChanged(ctx *log.Entry, i interface{}, i2 interface{}) (bool, error) {
	return false, nil
}

func NewMockDelegator() *MockDelegator {
	return &MockDelegator{}
}

func (m *MockDelegator) SetGetReturn(obj interface{}, err error) {
	m.obj = obj
	m.getErr = err
}

func (m *MockDelegator) Added(context.Context, interface{}) error {
	return nil
}
func (m *MockDelegator) Updated(context.Context, interface{}, interface{}) error {
	return nil
}
func (m *MockDelegator) Deleted(context.Context, interface{}) error {
	return nil
}
func (m *MockDelegator) UpdateProcessItemStatus(interface{}, string) error {
	return nil
}
func (m *MockDelegator) GetProcessItemStatus(interface{}) (string, error) {
	return "", nil
}
func (m *MockDelegator) LogValueOfAdmiralIoIgnore(interface{}) {
	return
}
func (m *MockDelegator) Get(context.Context, bool, interface{}) (interface{}, error) {
	return m.obj, m.getErr
}
