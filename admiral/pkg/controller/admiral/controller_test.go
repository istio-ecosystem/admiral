package admiral

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMonitoredDelegator_Added(t *testing.T) {
	td := &TestDelegator{}
	d := NewMonitoredDelegator(td, "test", "test")
	d.Added(context.Background(), nil)

	assert.True(t, td.AddedInvoked)
	assert.False(t, td.DeleteInvoked)
	assert.False(t, td.UpdatedInvoked)
}

func TestMonitoredDelegator_Deleted(t *testing.T) {
	td := &TestDelegator{}
	d := NewMonitoredDelegator(td, "test", "test")
	d.Deleted(context.Background(), nil)

	assert.False(t, td.AddedInvoked)
	assert.True(t, td.DeleteInvoked)
	assert.False(t, td.UpdatedInvoked)
}

func TestMonitoredDelegator_Updated(t *testing.T) {
	td := &TestDelegator{}
	d := NewMonitoredDelegator(td, "test", "test")
	d.Updated(context.Background(), nil, nil)

	assert.False(t, td.AddedInvoked)
	assert.False(t, td.DeleteInvoked)
	assert.True(t, td.UpdatedInvoked)
}

type TestDelegator struct {
	AddedInvoked   bool
	UpdatedInvoked bool
	DeleteInvoked  bool
}

func (t *TestDelegator) Added(context.Context, interface{}) {
	t.AddedInvoked = true
}

func (t *TestDelegator) Updated(context.Context, interface{}, interface{}) {
	t.UpdatedInvoked = true
}

func (t *TestDelegator) Deleted(context.Context, interface{}) {
	t.DeleteInvoked = true
}
