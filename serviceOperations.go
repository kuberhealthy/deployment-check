package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createServiceAndWait creates the service and waits for a cluster IP.
func (r *CheckRunner) createServiceAndWait(ctx context.Context, labels map[string]string) (*corev1.Service, error) {
	// Build the service manifest.
	serviceConfig := r.createServiceConfig(labels)
	log.Infoln("Created service resource.")

	// Create the service in the cluster.
	service, err := r.client.CoreV1().Services(r.cfg.CheckNamespace).Create(ctx, serviceConfig, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("service creation returned nil")
	}
	log.Infoln("Created service in", service.Namespace, "namespace:", service.Name)

	// Start a watch for the service to become available.
	watcher, err := r.client.CoreV1().Services(r.cfg.CheckNamespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		FieldSelector: "metadata.name=" + service.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch service: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case event := <-watcher.ResultChan():
			serviceEvent, ok := event.Object.(*corev1.Service)
			if !ok {
				log.Debugln("Got a watch event for a non-service object -- ignoring.")
				continue
			}
			if serviceAvailable(serviceEvent) {
				return serviceEvent, nil
			}
		case <-ctx.Done():
			cleanupErr := r.cleanup(ctx)
			if cleanupErr != nil {
				return nil, fmt.Errorf("failed to clean up after service create: %w", cleanupErr)
			}
			return nil, fmt.Errorf("context expired while waiting for service to create")
		}
	}
}

// deleteServiceAndWait deletes the service and waits for removal.
func (r *CheckRunner) deleteServiceAndWait(ctx context.Context) error {
	// Attempt a foreground delete with a short grace period.
	err := r.deleteService(ctx)
	if err != nil {
		log.Infoln("Could not delete service:", r.cfg.CheckServiceName)
	}

	// Loop until the service is no longer present.
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out while waiting for service to delete")
		default:
			log.Debugln("Delete service and wait has not yet timed out.")
		}

		log.Debugln("Waiting 5 seconds before trying again.")
		time.Sleep(time.Second * 5)

		serviceList, listErr := r.client.CoreV1().Services(r.cfg.CheckNamespace).List(ctx, metav1.ListOptions{
			FieldSelector: "metadata.name=" + r.cfg.CheckServiceName,
		})
		if listErr != nil {
			log.Errorln("Error listing services:", listErr.Error())
			continue
		}

		serviceExists := false
		for _, svc := range serviceList.Items {
			if svc.GetName() == r.cfg.CheckServiceName {
				serviceExists = true
				deleteErr := r.deleteService(ctx)
				if deleteErr != nil {
					log.Errorln("Error deleting service", r.cfg.CheckServiceName+":", deleteErr.Error())
				}
				break
			}
		}

		if !serviceExists {
			return nil
		}
	}
}

// deleteService issues the delete call for the service resource.
func (r *CheckRunner) deleteService(ctx context.Context) error {
	// Prepare foreground delete options.
	deletePolicy := metav1.DeletePropagationForeground
	graceSeconds := int64(1)
	deleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &graceSeconds,
		PropagationPolicy:  &deletePolicy,
	}

	// Issue the delete request.
	log.Infoln("Attempting to delete service", r.cfg.CheckServiceName, "in", r.cfg.CheckNamespace, "namespace.")
	return r.client.CoreV1().Services(r.cfg.CheckNamespace).Delete(ctx, r.cfg.CheckServiceName, deleteOpts)
}

// findPreviousService checks whether a prior service exists in the namespace.
func (r *CheckRunner) findPreviousService(ctx context.Context) (bool, error) {
	// List services in the target namespace.
	log.Infoln("Attempting to find previously created service(s) belonging to this check.")
	serviceList, err := r.client.CoreV1().Services(r.cfg.CheckNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	if serviceList == nil {
		return false, errors.New("received empty list of services")
	}
	log.Debugln("Found", len(serviceList.Items), "service(s).")

	// Scan for a matching service name.
	for _, svc := range serviceList.Items {
		if svc.Name == r.cfg.CheckServiceName {
			log.Infoln("Found an old service belonging to this check:", svc.Name)
			return true, nil
		}
	}

	log.Infoln("Did not find any old service(s) belonging to this check.")
	return false, nil
}

// getServiceClusterIP fetches the cluster IP for the service.
func (r *CheckRunner) getServiceClusterIP(ctx context.Context, service *corev1.Service) (string, error) {
	// Validate the service input.
	if service == nil {
		return "", fmt.Errorf("service reference was nil")
	}

	// Fetch the latest service to ensure cluster IP is populated.
	svc, err := r.client.CoreV1().Services(r.cfg.CheckNamespace).Get(ctx, r.cfg.CheckServiceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to fetch service for cluster IP: %w", err)
	}
	if svc == nil {
		return "", fmt.Errorf("failed to get service, received nil")
	}

	// Return the cluster IP when present.
	if len(svc.Spec.ClusterIP) != 0 {
		log.Infoln("Found service cluster IP address:", svc.Spec.ClusterIP)
		return svc.Spec.ClusterIP, nil
	}

	return "", fmt.Errorf("service cluster IP address is empty")
}

// serviceAvailable reports whether the service has a cluster IP assigned.
func serviceAvailable(service *corev1.Service) bool {
	// Guard against nil service references.
	if service == nil {
		return false
	}

	// Validate the cluster IP field.
	if len(service.Spec.ClusterIP) != 0 {
		log.Infoln("Cluster IP found:", service.Spec.ClusterIP)
		return true
	}

	return false
}
