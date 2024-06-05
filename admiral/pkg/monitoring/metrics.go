package monitoring

import (
	"context"
	"log"
	"reflect"

	api "go.opentelemetry.io/otel/metric"
)

// Metric interface for abstracting the different operations
// for the various metric types provided by open telemetry
type Metric interface {
	Increment(attributes api.MeasurementOption)
	Name() string
}

// NewCounter returns a new counter
func NewCounter(name, description string, opts ...Options) Metric {
	o := createOptions(opts...)
	return newFloat64Counter(name, description, o)
}

type counter struct {
	name         string
	description  string
	ctx          context.Context
	int64Counter api.Int64Counter
}

// Increment increases the value of the counter by 1, and adds the provided attributes
func (c *counter) Increment(attributes api.MeasurementOption) {
	c.int64Counter.Add(c.ctx, 1, attributes)
}

// Name returns the name of the metric
func (c *counter) Name() string {
	return c.name
}

func newFloat64Counter(name, description string, opts *options) *counter {
	ctx := context.TODO()
	meter := defaultMeter
	if reflect.ValueOf(opts.meter).IsValid() {
		meter = opts.meter
	}
	int64Counter, err := meter.Int64Counter(
		name,
		api.WithUnit("1"),
		api.WithDescription(description),
	)
	if err != nil {
		log.Fatalf("error creating int64 counter: %v", err)
	}
	return &counter{
		name:         name,
		description:  description,
		ctx:          ctx,
		int64Counter: int64Counter,
	}
}
