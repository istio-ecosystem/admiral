package util

import (
	"strings"
	"time"
)

type AdmiralState struct {
	ReadOnly           bool
	IsStateInitialized bool
}

var (
	CurrentAdmiralState AdmiralState
)

func IsAdmiralReadOnly() bool {
	return CurrentAdmiralState.ReadOnly
}

// ResyncIntervals defines the different reconciliation intervals
// for kubernetes operators
type ResyncIntervals struct {
	UniversalReconcileInterval time.Duration
	SeAndDrReconcileInterval   time.Duration
}

func GetPortProtocol(name string) string {
	var protocol = Http
	if strings.Index(name, GrpcWeb) == 0 {
		protocol = GrpcWeb
	} else if strings.Index(name, Grpc) == 0 {
		protocol = Grpc
	} else if strings.Index(name, Http2) == 0 {
		protocol = Http2
	}
	return protocol
}
