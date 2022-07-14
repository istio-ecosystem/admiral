package common

import (
	"github.com/prometheus/client_golang/prometheus"
	"sync"
)

const (
	ClustersMonitoredMetricName    = "clusters_monitored"
	EventsProcessedTotalMetricName = "events_processed_total"

	AddEventLabelValue    = "add"
	UpdateEventLabelValue = "update"
	DeleteEventLabelValue = "delete"
)

var (
	metricsOnce          sync.Once
	RemoteClustersMetric Gauge
	EventsProcessed      Counter
)

type Gauge interface {
	With(labelValues ...string) Gauge
	Set(value float64)
}

type Counter interface {
	With(labelValues ...string) Counter
	Inc()
}

/*
InitializeMetrics depends on AdmiralParams for metrics enablement.
*/
func InitializeMetrics() {
	metricsOnce.Do(func() {
		RemoteClustersMetric = NewGaugeFrom(ClustersMonitoredMetricName, "Gauge for the clusters monitored by Admiral", []string{})
		EventsProcessed = NewCounterFrom(EventsProcessedTotalMetricName, "Counter for the events processed by Admiral", []string{"cluster", "object_type", "event_type"})
	})
}

func NewGaugeFrom(name string, help string, labelNames []string) Gauge {
	if !GetMetricsEnabled() {
		return &NoopGauge{}
	}
	opts := prometheus.GaugeOpts{Name: name, Help: help}
	g := prometheus.NewGaugeVec(opts, labelNames)
	prometheus.MustRegister(g)
	return &PromGauge{g, labelNames}
}

func NewCounterFrom(name string, help string, labelNames []string) Counter {
	if !GetMetricsEnabled() {
		return &NoopCounter{}
	}
	opts := prometheus.CounterOpts{Name: name, Help: help}
	c := prometheus.NewCounterVec(opts, labelNames)
	prometheus.MustRegister(c)
	return &PromCounter{c, labelNames}
}

type NoopGauge struct{}
type NoopCounter struct{}

type PromGauge struct {
	g   *prometheus.GaugeVec
	lvs []string
}

type PromCounter struct {
	c   *prometheus.CounterVec
	lvs []string
}

func (g *PromGauge) With(labelValues ...string) Gauge {
	g.lvs = append([]string{}, labelValues...)

	return g
}

func (g *PromGauge) Set(value float64) {
	g.g.WithLabelValues(g.lvs...).Set(value)
}

func (c *PromCounter) With(labelValues ...string) Counter {
	c.lvs = append([]string{}, labelValues...)

	return c
}

func (c *PromCounter) Inc() {
	c.c.WithLabelValues(c.lvs...).Inc()
}

func (g *NoopGauge) Set(float64)          {}
func (g *NoopGauge) With(...string) Gauge { return g }

func (g *NoopCounter) Inc()                   {}
func (g *NoopCounter) With(...string) Counter { return g }
