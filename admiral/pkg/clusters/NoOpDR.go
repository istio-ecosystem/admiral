package clusters

import "fmt"

/*
Default implementation of the interface defined for DR
*/

type NoOPStateChecker struct {}

func (NoOPStateChecker) shouldRunOnIndependentGoRoutine() bool{
	return false;
}

func (NoOPStateChecker) runStateCheck(as *AdmiralState){
	fmt.Print("NoOP State Checker called. Marking Admiral state as Read/Write enabled")
	as.ReadOnly = READ_WRITE_ENABLED
}

