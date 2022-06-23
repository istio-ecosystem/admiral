package clusters

import (
	"context"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"strings"
)
const  READ_WRITE_ENABLED = false
const READ_ONLY_ENABLED = true;

type AdmiralState struct {
	ReadOnly  bool
}

type AdmiralStateChecker interface {
	runStateCheck(as *AdmiralState,ctx context.Context)
	shouldRunOnIndependentGoRoutine() bool
}
/*
Utility function to start Admiral DR checks.
DR checks can be run either on the main go routine or a new go routine
*/
func RunAdmiralStateCheck(asc AdmiralStateChecker, as *AdmiralState,ctx context.Context){
	log.Infof("Starting DR checks")
	if asc.shouldRunOnIndependentGoRoutine() {
		log.Info("Starting Admiral State Checker  on a new Go Routine")
		go asc.runStateCheck(as,ctx)
	}else {
		log.Infof("Starting Admiral State Checker on existing Go Routine")
		asc.runStateCheck(as,ctx)
	}
}

/*
utility function to identify the Admiral DR implementation based on the program parameters
*/
func startAdmiralStateChecker (params common.AdmiralParams,as *AdmiralState,ctx context.Context){
	var  admiralStateChecker AdmiralStateChecker
	switch  strings.ToLower(params.AdmiralStateCheckerName) {
	case "noopstatechecker":
		admiralStateChecker = NoOPStateChecker{}
	default:
		admiralStateChecker = NoOPStateChecker{}
	}
	RunAdmiralStateCheck(admiralStateChecker,as,ctx)
}