package main

import (
	"context"

	nodecheck "github.com/kuberhealthy/kuberhealthy/v3/pkg/nodecheck"
	log "github.com/sirupsen/logrus"
)

// waitForKuberhealthyReady ensures the reporting endpoint is reachable from this pod.
func (r *CheckRunner) waitForKuberhealthyReady(ctx context.Context) error {
	// Log the preflight state before the check work starts.
	log.Infoln("Waiting for Kuberhealthy endpoint to be reachable.")

	// Ask the shared nodecheck helper to verify connectivity.
	err := nodecheck.WaitForKuberhealthy(ctx)
	if err != nil {
		return err
	}

	return nil
}
