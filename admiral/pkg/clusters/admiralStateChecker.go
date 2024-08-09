package clusters

// admiralStateChecker.go

import (
	"context"
	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	"strings"

	log "github.com/sirupsen/logrus"
)

type AdmiralStateChecker interface {
	runStateCheck(ctx context.Context)
	shouldRunOnIndependentGoRoutine() bool
	initStateCache(interface{}) error
}

/*
Utility function to start Admiral DR checks.
DR checks can be run either on the main go routine or a new go routine
*/
func RunAdmiralStateCheck(ctx context.Context, stateChecker string, asc AdmiralStateChecker) {
	log.Infof("Starting %s state checker", stateChecker)
	if asc.shouldRunOnIndependentGoRoutine() {
		log.Infof("Starting %s state checker on a new Go Routine", stateChecker)
		go asc.runStateCheck(ctx)
	} else {
		log.Infof("Starting %s state checker on existing Go Routine", stateChecker)
		asc.runStateCheck(ctx)
	}
}

/*
utility function to identify the Admiral DR implementation based on the program parameters
*/
func initAdmiralStateChecker(ctx context.Context, stateChecker string, stateConfigFilePath string) AdmiralStateChecker {
	log.Printf("starting state checker for: %s", stateChecker)
	var admiralStateChecker AdmiralStateChecker
	var err error
	switch strings.ToLower(stateChecker) {
	// Add entries for your custom Disaster Recovery state checkers below
	// case "checker":
	// admiralStateChecker  = customChecker{}
	case ignoreIdentityChecker:
		admiralStateChecker, err = NewIgnoreIdentityStateChecker(stateConfigFilePath, NewDynamoClient)
		if err != nil {
			log.Fatalf("failed to configure %s state checker, err: %v", ignoreIdentityChecker, err)
		}
	case drStateChecker:
		admiralStateChecker = admiralReadWriteLeaseStateChecker{stateConfigFilePath}
	default:
		admiralStateChecker = NoOPStateChecker{}
	}
	return admiralStateChecker
}

/*
Default implementation of the interface defined for DR
*/
type NoOPStateChecker struct{}

func (NoOPStateChecker) shouldRunOnIndependentGoRoutine() bool {
	return false
}

func (NoOPStateChecker) initStateCache(cache interface{}) error {
	return nil
}

func (NoOPStateChecker) runStateCheck(ctx context.Context) {
	log.Info("NoOP State Checker called. Marking Admiral state as Read/Write enabled")
	commonUtil.CurrentAdmiralState.ReadOnly = ReadWriteEnabled
	commonUtil.CurrentAdmiralState.IsStateInitialized = StateInitialized
}
