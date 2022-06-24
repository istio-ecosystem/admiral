package clusters

import (
	"context"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
	"strings"
)
const  ReadWriteEnabled = false
const ReadOnlyEnabled = true;

type AdmiralState struct {
	ReadOnly  bool
}

type AdmiralStateChecker interface {
	runStateCheck(ctx context.Context,as *AdmiralState)
	shouldRunOnIndependentGoRoutine() bool
}
/*
Utility function to start Admiral DR checks.
DR checks can be run either on the main go routine or a new go routine
*/
func RunAdmiralStateCheck(ctx context.Context,asc AdmiralStateChecker, as *AdmiralState){
	log.Infof("Starting DR checks")
	if asc.shouldRunOnIndependentGoRoutine() {
		log.Info("Starting Admiral State Checker  on a new Go Routine")
		go asc.runStateCheck(ctx,as)
	}else {
		log.Infof("Starting Admiral State Checker on existing Go Routine")
		asc.runStateCheck(ctx,as)
	}
}

/*
utility function to identify the Admiral DR implementation based on the program parameters
*/
func startAdmiralStateChecker (ctx context.Context,params common.AdmiralParams,as *AdmiralState,){
	var  admiralStateChecker AdmiralStateChecker
	switch  strings.ToLower(params.AdmiralStateCheckerName) {
	case "noopstatechecker":
		admiralStateChecker = NoOPStateChecker{}
	default:
		admiralStateChecker = NoOPStateChecker{}
	}
	RunAdmiralStateCheck(ctx,admiralStateChecker,as)
}