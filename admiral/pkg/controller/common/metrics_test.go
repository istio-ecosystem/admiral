package common

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewGaugeFrom(t *testing.T) {
	type args struct {
		prom bool
		Name string
		Help string
	}
	tc := []struct {
		name string
		args args
		want Gauge
	}{
		{
			"Should return a Prometheus gauge",
			args{true, "gauge", ""},
			&PromGauge{prometheus.NewGauge(prometheus.GaugeOpts{Name: "gauge", Help: ""})},
		},
		{
			"Should return a Noop gauge",
			args{false, "gauge", ""},
			&Noop{},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			SetEnablePrometheus(tt.args.prom)
			actual := NewGaugeFrom(tt.args.Name, tt.args.Help)
			assert.Equal(t, tt.want, actual, "want: %#v, got: %#v", tt.want, actual)
		})
	}
}
