package clusters

import "fmt"

type NoOPStateChecker struct {}

func (NoOPStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return false;
}

func (NoOPStateChecker) stateCheckBasedDREnabled() bool {
	return false;
}

func (NoOPStateChecker) getStateCheckerName()  string {
	return "noopstatechecker"
}

func (NoOPStateChecker) runStateCheck(as *AdmiralState){
	fmt.Print("NoOP State Checker called. Marking Admiral state as Read/Write enabled")
	as.ReadOnly = READ_WRITE_ENABLED
}

