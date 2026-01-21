package main

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
)

// CheckRunner bundles dependencies and configuration for running the deployment check.
type CheckRunner struct {
	// cfg stores the parsed check configuration.
	cfg *CheckConfig
	// client provides typed Kubernetes API access.
	client *kubernetes.Clientset
	// now pins a timestamp for resource labeling during a run.
	now time.Time
}

// newCheckRunner builds a runner with configuration and Kubernetes access.
func newCheckRunner(cfg *CheckConfig, client *kubernetes.Clientset, now time.Time) *CheckRunner {
	// Assemble the runner that will execute the check steps.
	return &CheckRunner{
		cfg:    cfg,
		client: client,
		now:    now,
	}
}

// run executes the full deployment check flow and reports back to Kuberhealthy.
func (r *CheckRunner) run(ctx context.Context) error {
	// Wait for Kuberhealthy to accept reports before doing any work.
	err := r.waitForKuberhealthyReady(ctx)
	if err != nil {
		return err
	}

	// Clear any leftovers from prior runs.
	err = r.cleanupOrphans(ctx)
	if err != nil {
		return err
	}

	// Capture the run deadline for create/update monitoring.
	deadline := time.Now().Add(r.cfg.CheckTimeLimit)

	// Create a deployment for the check.
	deploymentResult, err := r.createDeploymentAndWait(ctx, deadline)
	if err != nil {
		return err
	}

	// Create a service for the deployment.
	serviceResult, err := r.createServiceAndWait(ctx, deploymentResult.Spec.Template.Labels)
	if err != nil {
		cleanupErr := r.cleanup(ctx)
		if cleanupErr != nil {
			return fmt.Errorf("service creation failed: %w; cleanup error: %w", err, cleanupErr)
		}
		return fmt.Errorf("service creation failed: %w", err)
	}

	// Fetch the service IP that will be used for HTTP checks.
	serviceIP, err := r.getServiceClusterIP(ctx, serviceResult)
	if err != nil {
		return fmt.Errorf("service lookup failed: %w", err)
	}

	// Validate a 200 response from the service.
	err = r.requestServiceEndpoint(ctx, serviceIP)
	if err != nil {
		cleanupErr := r.cleanup(ctx)
		if cleanupErr != nil {
			return fmt.Errorf("service request failed: %w; cleanup error: %w", err, cleanupErr)
		}
		return fmt.Errorf("service request failed: %w", err)
	}

	// Handle optional rolling updates.
	if r.cfg.RollingUpdate {
		err = r.rollDeploymentAndVerify(ctx)
		if err != nil {
			return err
		}
	}

	// Clean up resources after a successful run.
	err = r.cleanup(ctx)
	if err != nil {
		return err
	}

	return nil
}
