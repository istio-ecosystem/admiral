package clusters

import (
	"context"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	log "github.com/sirupsen/logrus"
)
const  ReadWriteEnabled = false
const ReadOnlyEnabled = true;
const StateNotInitialized = false;
const StateInitialized =true;

type AdmiralState struct {
	ReadOnly  bool
	IsStateInitialized bool
}
var CurrentAdmiralState AdmiralState

type AdmiralStateChecker interface {
	runStateCheck(ctx context.Context)
	shouldRunOnIndependentGoRoutine() bool
}
/*
Utility function to start Admiral DR checks.
DR checks can be run either on the main go routine or a new go routine
*/
func RunAdmiralStateCheck(ctx context.Context,asc AdmiralStateChecker){
	log.Infof("Starting Disaster Recovery state checks")
	if asc.shouldRunOnIndependentGoRoutine() {
		log.Info("Starting Admiral State Checker  on a new Go Routine")
		go asc.runStateCheck(ctx)
	}else {
		log.Infof("Starting Admiral State Checker on existing Go Routine")
		asc.runStateCheck(ctx)
	}
}

/*
utility function to identify the Admiral DR implementation based on the program parameters
*/
func startAdmiralStateChecker (ctx context.Context,params common.AdmiralParams){
	var  admiralStateChecker AdmiralStateChecker
	switch  params.AdmiralStateCheckerName {
/*
     Add entries for your custom Disaster Recovery state checkers below
     case "keywordforsomecustomchecker":
		admiralStateChecker  = customChecker{}
*/
	default:
		admiralStateChecker = NoOPStateChecker{}
	}
	RunAdmiralStateCheck(ctx,admiralStateChecker)
}