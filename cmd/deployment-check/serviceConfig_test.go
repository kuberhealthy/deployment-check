package main

import "testing"

// TestCreateServiceConfig validates service metadata and ports.
func TestCreateServiceConfig(t *testing.T) {
	// Build a runner with default configuration.
	runner := buildTestRunner()

	// Define a set of image inputs to validate.
	cases := []string{"test-image:latest", "nginx:latest", "nginx:test"}

	// Validate service fields for each image.
	for _, image := range cases {
		deploymentConfig := runner.createDeploymentConfig(image)

		if deploymentConfig.Spec.Selector == nil {
			t.Fatalf("deployment config was created without selectors: %v", deploymentConfig.Spec.Selector)
		}

		if len(deploymentConfig.Spec.Selector.MatchLabels) == 0 {
			t.Fatalf("deployment config was created without selector match labels: %v", deploymentConfig.Spec.Selector)
		}

		serviceConfig := runner.createServiceConfig(deploymentConfig.Spec.Selector.MatchLabels)

		if len(serviceConfig.Name) == 0 {
			t.Fatalf("nil service name: %s", serviceConfig.Name)
		}

		if len(serviceConfig.Namespace) == 0 {
			t.Fatalf("nil service namespace: %s", serviceConfig.Namespace)
		}

		if len(serviceConfig.Spec.Ports) == 0 {
			t.Fatalf("no ports created for service: %d ports", len(serviceConfig.Spec.Ports))
		}
	}
}
