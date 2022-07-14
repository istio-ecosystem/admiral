package clusters

import (
	"context"
	"fmt"
)

/*
Default implementation of the interface defined for DR
*/

type NoOPStateChecker struct {}

func (NoOPStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return false;
}

func (NoOPStateChecker) runStateCheck(ctx context.Context){
	fmt.Print("NoOP State Checker called. Marking Admiral state as Read/Write enabled")
	CurrentAdmiralState = AdmiralState{ReadWriteEnabled}
}

