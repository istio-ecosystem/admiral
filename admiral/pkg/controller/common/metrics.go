package common

import "github.com/prometheus/client_golang/prometheus"

const ClustersMonitoredMetricName = "clusters_monitored"

type Gauge interface {
	Set(value float64)
}

func NewGaugeFrom(name string, help string) Gauge {
	if !GetMetricsEnabled() {
		return &Noop{}
	}
	opts := prometheus.GaugeOpts{Name: name, Help: help}
	g := prometheus.NewGauge(opts)
	prometheus.MustRegister(g)
	return &PromGauge{g}
}

type Noop struct{}

type PromGauge struct {
	g prometheus.Gauge
}

func (g *PromGauge) Set(value float64) {
	g.g.Set(value)
}

func (g *Noop) Set(value float64) {}
