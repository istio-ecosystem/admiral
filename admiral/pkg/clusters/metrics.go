package clusters

import "github.com/istio-ecosystem/admiral/admiral/pkg/monitoring"

// List all metrics part of the clusters package here
var (
	configWriterMeter       = monitoring.NewMeter("config_writer")
	totalConfigWriterEvents = monitoring.NewCounter(
		"total_config_write_invocations",
		"total number of times config writer was invoked",
		monitoring.WithMeter(configWriterMeter))
)
