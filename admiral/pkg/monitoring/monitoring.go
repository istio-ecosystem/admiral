package monitoring

import (
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

var (
	meterName    = "admiral_monitoring"
	exporter, _  = prometheus.New()
	provider     = metric.NewMeterProvider(metric.WithReader(exporter))
	defaultMeter = provider.Meter(meterName)
)

func InitializeMonitoring() error {
	return nil
}

// Options accepts a pointer to options. It is used
// to update the options by calling an array of functions
type Options func(*options)

// NewMeter creates a new meter which defines the metric scope
func NewMeter(meterName string) api.Meter {
	return provider.Meter(meterName)
}

// WithMeter configures the given Meter
func WithMeter(meter api.Meter) Options {
	return func(opts *options) {
		opts.meter = meter
	}
}

type options struct {
	meter api.Meter
	unit  string
}

func createOptions(opts ...Options) *options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
