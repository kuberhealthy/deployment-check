package main

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// TestCreateContainerConfig validates container fields used by the deployment check.
func TestCreateContainerConfig(t *testing.T) {
	// Build a runner with default configuration.
	runner := buildTestRunner()

	// Define a set of image inputs to validate.
	cases := []string{"test-image:latest", "nginx:latest", "nginx:test"}

	// Validate the generated container configuration for each image.
	for _, image := range cases {
		containerConfig := runner.createContainerConfig(image)

		if len(containerConfig.Name) == 0 {
			t.Fatalf("nil container name: %s", containerConfig.Name)
		}

		if len(containerConfig.Image) == 0 {
			t.Fatalf("nil container image: %s", containerConfig.Image)
		}

		if containerConfig.Image != image {
			t.Fatalf("expected container image to be %s but got: %s", image, containerConfig.Image)
		}

		if len(containerConfig.ImagePullPolicy) == 0 {
			t.Fatalf("nil image pull policy: %s", containerConfig.ImagePullPolicy)
		}

		if len(containerConfig.Ports) == 0 {
			t.Fatalf("no ports given for container: found %d ports", len(containerConfig.Ports))
		}

		if containerConfig.LivenessProbe == nil {
			t.Fatalf("nil container liveness probe: %v", containerConfig.LivenessProbe)
		}

		if containerConfig.ReadinessProbe == nil {
			t.Fatalf("nil container readiness probe: %v", containerConfig.ReadinessProbe)
		}
	}
}

// TestCreateDeploymentConfig validates deployment metadata and selectors.
func TestCreateDeploymentConfig(t *testing.T) {
	// Build a runner with default configuration.
	runner := buildTestRunner()

	// Define a set of image inputs to validate.
	cases := []string{"test-image:latest", "nginx:latest", "nginx:test"}

	// Validate deployment fields for each image.
	for _, image := range cases {
		deploymentConfig := runner.createDeploymentConfig(image)

		if len(deploymentConfig.ObjectMeta.Name) == 0 {
			t.Fatalf("nil deployment object meta name: %s", deploymentConfig.ObjectMeta.Name)
		}

		if len(deploymentConfig.ObjectMeta.Namespace) == 0 {
			t.Fatalf("nil deployment object meta namespace: %s", deploymentConfig.ObjectMeta.Namespace)
		}

		if deploymentConfig.Spec.Replicas == nil {
			t.Fatalf("deployment config missing replicas")
		}

		if *deploymentConfig.Spec.Replicas < 1 {
			t.Fatalf("deployment config was created with less than 1 replica: %d", *deploymentConfig.Spec.Replicas)
		}

		if deploymentConfig.Spec.Selector == nil {
			t.Fatalf("deployment config was created without selectors: %v", deploymentConfig.Spec.Selector)
		}

		if len(deploymentConfig.Spec.Selector.MatchLabels) == 0 {
			t.Fatalf("deployment config was created without selector match labels: %v", deploymentConfig.Spec.Selector)
		}
	}
}

// buildTestRunner creates a runner with defaults for unit tests and check configuration.
func buildTestRunner() *CheckRunner {
	// Build a minimal config with defaults needed for generation functions.
	cfg := &CheckConfig{
		CheckDeploymentName:          defaultCheckDeploymentName,
		CheckServiceName:             defaultCheckServiceName,
		CheckContainerPort:           defaultCheckContainerPort,
		CheckLoadBalancerPort:        defaultCheckLoadBalancerPort,
		CheckNamespace:               defaultCheckNamespace,
		CheckDeploymentReplicas:      defaultCheckDeploymentReplicas,
		CheckServiceAccount:          defaultCheckServiceAccount,
		MillicoreRequest:             defaultMillicoreRequest,
		MillicoreLimit:               defaultMillicoreLimit,
		MemoryRequest:                defaultMemoryRequest,
		MemoryLimit:                  defaultMemoryLimit,
		AdditionalEnvVars:            map[string]string{},
		CheckDeploymentNodeSelectors: map[string]string{},
		CheckDeploymentTolerations:   []corev1.Toleration{},
	}

	// Create the runner with a fixed timestamp.
	runner := newCheckRunner(cfg, nil, time.Now())
	return runner
}
