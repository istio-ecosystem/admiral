package admiral

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMonitoredDelegator_Added(t *testing.T) {
	td := &TestDelegator{}
	d := NewMonitoredDelegator(td, "test", "test")
	d.Added(nil)

	assert.True(t, td.AddedInvoked)
	assert.False(t, td.DeleteInvoked)
	assert.False(t, td.UpdatedInvoked)
}

func TestMonitoredDelegator_Deleted(t *testing.T) {
	td := &TestDelegator{}
	d := NewMonitoredDelegator(td, "test", "test")
	d.Deleted(nil)

	assert.False(t, td.AddedInvoked)
	assert.True(t, td.DeleteInvoked)
	assert.False(t, td.UpdatedInvoked)
}

func TestMonitoredDelegator_Updated(t *testing.T) {
	td := &TestDelegator{}
	d := NewMonitoredDelegator(td, "test", "test")
	d.Updated(nil, nil)

	assert.False(t, td.AddedInvoked)
	assert.False(t, td.DeleteInvoked)
	assert.True(t, td.UpdatedInvoked)
}

type TestDelegator struct {
	AddedInvoked   bool
	UpdatedInvoked bool
	DeleteInvoked  bool
}

func (t *TestDelegator) Added(obj interface{}) {
	t.AddedInvoked = true
}

func (t *TestDelegator) Updated(obj interface{}, oldObj interface{}) {
	t.UpdatedInvoked = true
}

func (t *TestDelegator) Deleted(obj interface{}) {
	t.DeleteInvoked = true
}
