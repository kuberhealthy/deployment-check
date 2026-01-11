package main

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// createKubeClient builds a Kubernetes clientset for in-cluster or kubeconfig use.
func createKubeClient(kubeConfigPath string) (*kubernetes.Clientset, error) {
	// Attempt in-cluster configuration first.
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig for local development.
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create kubeconfig: %w", err)
		}
	}

	// Build the clientset for typed API access.
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return clientset, nil
}
