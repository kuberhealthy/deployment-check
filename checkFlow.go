package main

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cleanup removes the deployment and service created by the check.
func (r *CheckRunner) cleanup(ctx context.Context) error {
	// Track aggregated errors for cleanup.
	resultErr := ""

	// Delete the service first.
	log.Infoln("Cleaning up deployment and service.")
	serviceErr := r.deleteServiceAndWait(ctx)
	if serviceErr != nil {
		log.Errorln("Error cleaning up service:", serviceErr.Error())
		resultErr = resultErr + "error cleaning up service: " + serviceErr.Error()
	}

	// Delete the deployment second.
	deploymentErr := r.deleteDeploymentAndWait(ctx)
	if deploymentErr != nil {
		log.Errorln("Error cleaning up deployment:", deploymentErr.Error())
		if len(resultErr) != 0 {
			resultErr = resultErr + " | "
		}
		resultErr = resultErr + "error cleaning up deployment: " + deploymentErr.Error()
	}

	// Return a combined error if needed.
	if len(resultErr) != 0 {
		return fmt.Errorf("%s", resultErr)
	}

	log.Infoln("Finished clean up process.")
	return nil
}

// cleanupOrphans removes stale resources before starting a new run.
func (r *CheckRunner) cleanupOrphans(ctx context.Context) error {
	// Bound the cleanup with a timeout to avoid hanging.
	cleanupTimeout := time.After(time.Minute * 2)

	// Find any previous resources created by this check.
	serviceExists, err := r.findPreviousService(ctx)
	if err != nil {
		log.Warnln("Failed to find previous service:", err.Error())
	}
	if serviceExists {
		log.Infoln("Found previous service.")
	}
	deploymentExists, err := r.findPreviousDeployment(ctx)
	if err != nil {
		log.Warnln("Failed to find previous deployment:", err.Error())
	}
	if deploymentExists {
		log.Infoln("Found previous deployment.")
	}

	// Clean up if anything was found.
	if serviceExists || deploymentExists {
		log.Infoln("Wiping all found orphaned resources belonging to this check.")
		cleanupDone := make(chan error, 1)
		go r.runCleanupAsync(ctx, cleanupDone)

		select {
		case cleanupErr := <-cleanupDone:
			return cleanupErr
		case <-ctx.Done():
			return fmt.Errorf("failed to perform pre-check cleanup within timeout")
		case <-cleanupTimeout:
			return fmt.Errorf("failed to perform pre-check cleanup within timeout")
		}
	}

	log.Infoln("Successfully cleaned up prior check resources.")
	return nil
}

// runCleanupAsync performs cleanup work in a goroutine.
func (r *CheckRunner) runCleanupAsync(ctx context.Context, resultChan chan<- error) {
	// Run cleanup and forward the result.
	cleanupErr := r.cleanup(ctx)
	resultChan <- cleanupErr
}

// rollDeploymentAndVerify performs the rolling update and validates the service again.
func (r *CheckRunner) rollDeploymentAndVerify(ctx context.Context) error {
	// Compute the deadline for rollout operations.
	deadline := time.Now().Add(r.cfg.CheckTimeLimit)

	// Update the deployment with the new image.
	updatedDeployment, err := r.updateDeploymentAndWait(ctx, deadline)
	if err != nil {
		return err
	}
	log.Infoln("Rolled deployment in", updatedDeployment.Namespace, "namespace:", updatedDeployment.Name)

	// Fetch the service cluster IP.
	service, err := r.client.CoreV1().Services(r.cfg.CheckNamespace).Get(ctx, r.cfg.CheckServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to fetch service after rolling update: %w", err)
	}
	serviceIP, err := r.getServiceClusterIP(ctx, service)
	if err != nil {
		return err
	}

	// Validate the service endpoint after rolling update.
	log.Infoln("Rolling update completed. Validating service endpoint again.")
	return r.requestServiceEndpoint(ctx, serviceIP)
}
