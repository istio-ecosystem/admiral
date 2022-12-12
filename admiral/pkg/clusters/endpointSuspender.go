package clusters

type EndpointSuspender interface {
	SuspendGeneration(identity string, environment string) bool
}
