package clusters

import (
	"context"
	log "github.com/sirupsen/logrus"
)

/*
Default implementation of the interface defined for DR
*/

type NoOPStateChecker struct {}

func (NoOPStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return false;
}

func (NoOPStateChecker) runStateCheck(ctx context.Context){
	log.Info("NoOP State Checker called. Marking Admiral state as Read/Write enabled")
	CurrentAdmiralState.ReadOnly = ReadWriteEnabled
	CurrentAdmiralState.IsStateInitialized = StateInitialized
}

