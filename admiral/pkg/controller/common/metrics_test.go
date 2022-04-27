package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

/*
Figure out a way to init admiral params again in the test. it is conflicting with common_test.go
*/
//func TestNewGaugeFromReturnsPrometheus(t *testing.T) {
//	InitializeConfig(AdmiralParams{PrometheusEnabled: true})
//	assert.Equalf(t, &PromGauge{g: prometheus.NewGauge(prometheus.GaugeOpts{Name: "sample-gauge", Help: ""})}, NewGaugeFrom("sample-gauge", ""), "NewGaugeFrom(%v, %v)", "sample-gauge", "")
//}

func TestNewGaugeFromReturnsNoop(t *testing.T) {
	assert.Equalf(t, &Noop{}, NewGaugeFrom("sample-gauge", ""), "NewGaugeFrom(%v, %v)", "sample-gauge", "")
}
