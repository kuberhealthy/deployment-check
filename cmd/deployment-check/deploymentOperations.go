package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	corev1typed "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	// errDeploymentCreatePod indicates a create-time pod error.
	errDeploymentCreatePod = errors.New("pod in the process of creating deployment")
	// errDeploymentUpdatePod indicates an update-time pod error.
	errDeploymentUpdatePod = errors.New("pod in the process of updating deployment")
)

// createDeploymentAndWait creates the deployment and waits for availability.
func (r *CheckRunner) createDeploymentAndWait(ctx context.Context, deadline time.Time) (*appsv1.Deployment, error) {
	// Build the deployment manifest.
	deploymentConfig := r.createDeploymentConfig(r.cfg.CheckImageURL)
	log.Infoln("Created deployment resource.")

	// Create the deployment.
	deployment, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Create(ctx, deploymentConfig, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}
	if deployment == nil {
		return nil, fmt.Errorf("deployment creation returned nil")
	}
	log.Infoln("Created deployment in", deployment.Namespace, "namespace:", deployment.Name)

	// Watch for pod errors in a background goroutine.
	ctxCreate, cancel := context.WithCancel(context.Background())
	defer cancel()
	podErrorChan := make(chan error, 1)
	go r.monitorDeploymentPodErrors(ctxCreate, deadline, 2, errDeploymentCreatePod, podErrorChan)

	// Wait for the deployment to become available.
	watcher, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		FieldSelector: "metadata.name=" + deployment.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch deployment: %w", err)
	}
	defer watcher.Stop()

	for {
		// Handle events, errors, or context cancellation.
		select {
		case event := <-watcher.ResultChan():
			deploymentEvent, ok := event.Object.(*appsv1.Deployment)
			if !ok {
				log.Infoln("Got a watch event for a non-deployment object -- ignoring.")
				continue
			}
			log.Debugln("Received an event watching for deployment changes:", deploymentEvent.Name, "got event", event.Type)
			if deploymentAvailable(deploymentEvent, r.cfg.CheckDeploymentReplicas) {
				return deploymentEvent, nil
			}
		case podErr := <-podErrorChan:
			if podErr != nil {
				return nil, r.decorateDeploymentError(ctx, "deployment create", podErr)
			}
		case <-ctx.Done():
			cleanupErr := r.cleanup(ctx)
			if cleanupErr != nil {
				return nil, fmt.Errorf("failed to clean up after deployment create: %w", cleanupErr)
			}
			return nil, r.decorateDeploymentError(ctx, "deployment create", fmt.Errorf("context expired while waiting for deployment to create"))
		}
	}
}

// updateDeploymentAndWait performs a rolling update and waits for completion.
func (r *CheckRunner) updateDeploymentAndWait(ctx context.Context, deadline time.Time) (*appsv1.Deployment, error) {
	// Fetch the current deployment to preserve resourceVersion.
	current, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Get(ctx, r.cfg.CheckDeploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deployment for update: %w", err)
	}
	if current == nil {
		return nil, fmt.Errorf("deployment lookup returned nil")
	}

	// Create the updated spec and apply the new image.
	updatedConfig := r.createDeploymentConfig(r.cfg.CheckImageURLRollTo)
	if len(updatedConfig.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("updated deployment config did not include containers")
	}

	// Copy the new template into the existing deployment to keep metadata intact.
	current.Spec.Template = updatedConfig.Spec.Template
	current.Spec.Replicas = updatedConfig.Spec.Replicas
	current.Spec.Strategy = updatedConfig.Spec.Strategy
	current.Spec.MinReadySeconds = updatedConfig.Spec.MinReadySeconds

	log.Infoln("Performing rolling-update on deployment", current.Name, "to ["+r.cfg.CheckImageURLRollTo+"]")

	// Submit the update.
	deployment, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update deployment: %w", err)
	}

	// Watch for pod errors in a background goroutine.
	ctxUpdate, cancel := context.WithCancel(context.Background())
	defer cancel()
	podErrorChan := make(chan error, 1)
	go r.monitorDeploymentPodErrors(ctxUpdate, deadline, 3, errDeploymentUpdatePod, podErrorChan)

	// Watch for the rolling update to complete.
	watcher, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		FieldSelector: "metadata.name=" + deployment.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch deployment update: %w", err)
	}
	defer watcher.Stop()

	for {
		// Wait for deployment status updates.
		select {
		case event := <-watcher.ResultChan():
			deploymentEvent, ok := event.Object.(*appsv1.Deployment)
			if !ok {
				log.Infoln("Got a watch event for a non-deployment object -- ignoring.")
				continue
			}
			log.Debugln("Received an event watching for deployment changes:", deploymentEvent.Name, "got event", event.Type)
			if rolledPodsAreReady(deploymentEvent, r.cfg.CheckDeploymentReplicas) {
				return deploymentEvent, nil
			}
		case podErr := <-podErrorChan:
			if podErr != nil {
				return nil, r.decorateDeploymentError(ctx, "deployment update", podErr)
			}
		case <-ctx.Done():
			cleanupErr := r.cleanup(ctx)
			if cleanupErr != nil {
				return nil, fmt.Errorf("failed to clean up after deployment update: %w", cleanupErr)
			}
			return nil, r.decorateDeploymentError(ctx, "deployment update", fmt.Errorf("context expired while waiting for deployment to update"))
		}
	}
}

// deleteDeploymentAndWait deletes the deployment and waits for removal.
func (r *CheckRunner) deleteDeploymentAndWait(ctx context.Context) error {
	// Attempt a foreground delete with a short grace period.
	err := r.deleteDeployment(ctx)
	if err != nil {
		log.Infoln("Could not delete deployment:", r.cfg.CheckDeploymentName)
	}

	// Loop until the deployment is no longer present.
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out while waiting for deployment to delete")
		default:
			log.Debugln("Delete deployment and wait has not yet timed out.")
		}

		log.Debugln("Waiting 5 seconds before trying again.")
		time.Sleep(time.Second * 5)

		deploymentList, listErr := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).List(ctx, metav1.ListOptions{
			FieldSelector: "metadata.name=" + r.cfg.CheckDeploymentName,
		})
		if listErr != nil {
			log.Errorln("Error listing deployments:", listErr.Error())
			continue
		}

		deploymentExists := false
		for _, deploy := range deploymentList.Items {
			if deploy.GetName() == r.cfg.CheckDeploymentName {
				deploymentExists = true
				deleteErr := r.deleteDeployment(ctx)
				if deleteErr != nil {
					log.Errorln("Error deleting deployment", r.cfg.CheckDeploymentName+":", deleteErr.Error())
				}
				break
			}
		}

		if !deploymentExists {
			return nil
		}
	}
}

// deleteDeployment issues the delete call for the deployment resource.
func (r *CheckRunner) deleteDeployment(ctx context.Context) error {
	// Prepare background delete options to avoid foreground finalizer stalls.
	deletePolicy := metav1.DeletePropagationBackground
	graceSeconds := int64(1)
	deleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &graceSeconds,
		PropagationPolicy:  &deletePolicy,
	}

	// Issue the delete request.
	log.Infoln("Attempting to delete deployment in", r.cfg.CheckNamespace, "namespace.")
	return r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Delete(ctx, r.cfg.CheckDeploymentName, deleteOpts)
}

// findPreviousDeployment checks whether a prior deployment exists in the namespace.
func (r *CheckRunner) findPreviousDeployment(ctx context.Context) (bool, error) {
	// List deployments in the target namespace.
	log.Infoln("Attempting to find previously created deployment(s) belonging to this check.")
	deploymentList, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	if deploymentList == nil {
		return false, errors.New("received empty list of deployments")
	}
	log.Debugln("Found", len(deploymentList.Items), "deployment(s)")

	// Scan for a matching deployment name.
	for _, deployment := range deploymentList.Items {
		if deployment.Name == r.cfg.CheckDeploymentName {
			log.Infoln("Found an old deployment belonging to this check:", deployment.Name)
			return true, nil
		}
	}

	log.Infoln("Did not find any old deployment(s) belonging to this check.")
	return false, nil
}

// monitorDeploymentPodErrors inspects pod states and events to surface deployment issues.
func (r *CheckRunner) monitorDeploymentPodErrors(ctx context.Context, deadline time.Time, divisor int, reason error, resultChan chan<- error) {
	// Loop until the context is canceled or an error is detected.
	for {
		select {
		case <-ctx.Done():
			log.Infoln("Deployment pod monitor exiting.")
			return
		default:
		}

		// Only start evaluating errors later in the run to allow for startup.
		if divisor > 0 && time.Until(deadline) < r.cfg.CheckTimeLimit/time.Duration(divisor) {
			log.Infoln("Capturing possible pod errors while deployment is in progress.")
			podErr := r.checkDeploymentPodEvent(ctx, reason)
			if podErr != nil {
				resultChan <- podErr
				return
			}
		}

		// Sleep briefly to avoid hammering the API.
		time.Sleep(time.Second * 2)
	}
}

// checkDeploymentPodEvent inspects pod and event states for deployment errors.
func (r *CheckRunner) checkDeploymentPodEvent(ctx context.Context, reason error) error {
	// List pods for the current deployment run.
	podList, err := r.client.CoreV1().Pods(r.cfg.CheckNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: deploymentLabelKey + "=" + deploymentLabelValueBase + fmt.Sprint(r.now.Unix()),
	})
	if err != nil {
		log.WithError(err).Errorln("Error listing deployment pods while waiting for readiness.")
		return err
	}

	// Inspect each pod and container status.
	for _, pod := range podList.Items {
		for _, containerStat := range pod.Status.ContainerStatuses {
			if containerStat.State.Waiting == nil {
				continue
			}
			if containerStat.State.Waiting.Reason == "" {
				continue
			}

			// Capture waiting errors that are not the normal ContainerCreating state.
			if !containerStat.Ready && containerStat.State.Waiting.Reason != "ContainerCreating" {
				err = fmt.Errorf("pod: %s node: %s container: %s reason: %s msg: %s",
					pod.Name,
					pod.Spec.NodeName,
					containerStat.Name,
					containerStat.State.Waiting.Reason,
					containerStat.State.Waiting.Message,
				)
				log.WithError(err).Errorln("Capturing unexpected container error.")
				return fmt.Errorf("pod state error: %s; stage: %w", err.Error(), reason)
			}

			// Inspect events associated with the pod for errors.
			deploymentPodEventList, eventErr := r.client.CoreV1().Events(r.cfg.CheckNamespace).List(context.Background(), metav1.ListOptions{
				FieldSelector: corev1typed.GetInvolvedObjectNameFieldLabel("v1") + "=" + pod.Name,
			})
			if eventErr != nil && !k8serrors.IsNotFound(eventErr) {
				return eventErr
			}

			// Track the most recent error event.
			eventReason := ""
			eventMsg := ""
			var recentEventTime time.Time
			for _, checkerPodEvent := range deploymentPodEventList.Items {
				checkerReason := strings.ToLower(checkerPodEvent.Reason)
				if !strings.Contains(checkerReason, "err") && !strings.Contains(checkerReason, "failed") && !strings.Contains(checkerReason, "backoff") {
					continue
				}
				if checkerPodEvent.LastTimestamp.Time.After(recentEventTime) {
					recentEventTime = checkerPodEvent.LastTimestamp.Time
					eventReason = checkerPodEvent.Reason
					eventMsg = checkerPodEvent.Message
				}
			}

			// Return the most recent event error if found.
			if len(eventReason) != 0 {
				err = fmt.Errorf("pod: %s node: %s container: %s reason: %s msg: %s",
					pod.Name,
					pod.Spec.NodeName,
					containerStat.Name,
					eventReason,
					eventMsg,
				)
				log.WithError(err).Errorln("Capturing unexpected pod event.")
				return fmt.Errorf("pod event error: %s; stage: %w", err.Error(), reason)
			}
		}

		// Surface pod failures at the pod phase level.
		if pod.Status.Phase == corev1.PodFailed {
			err = fmt.Errorf("pod: %s node: %s reason: %s msg: %s",
				pod.Name,
				pod.Spec.NodeName,
				pod.Status.Reason,
				pod.Status.Message,
			)
			log.WithError(err).Errorln("Pod in failed status.")
			return fmt.Errorf("pod failed: %s; stage: %w", err.Error(), reason)
		}
	}

	return nil
}

// deploymentAvailable checks status conditions for availability after create.
func deploymentAvailable(deployment *appsv1.Deployment, replicas int) bool {
	// Guard against nil inputs.
	if deployment == nil {
		return false
	}

	// Iterate conditions to find an available state.
	for _, condition := range deployment.Status.Conditions {
		if condition.Type != appsv1.DeploymentAvailable {
			continue
		}
		if condition.Status != corev1.ConditionTrue {
			continue
		}

		// Ensure expected replica counts are met.
		if deployment.Status.Replicas != int32(replicas) {
			continue
		}
		if deployment.Status.AvailableReplicas != int32(replicas) {
			continue
		}
		if deployment.Status.ReadyReplicas != int32(replicas) {
			continue
		}
		if deployment.Status.ObservedGeneration != 1 {
			continue
		}

		log.Infoln("Deployment is reporting", condition.Type, "with", condition.Status+".")
		log.Infoln(deployment.Status.AvailableReplicas, "deployment pods are ready and available.")
		return true
	}

	return false
}

// rolledPodsAreReady checks if updated pods are available after a rolling update.
func rolledPodsAreReady(deployment *appsv1.Deployment, replicas int) bool {
	// Guard against nil inputs.
	if deployment == nil {
		return false
	}

	// Confirm that rollout has reached the desired state.
	if deployment.Status.Replicas != int32(replicas) {
		return false
	}
	if deployment.Status.UpdatedReplicas != int32(replicas) {
		return false
	}
	if deployment.Status.AvailableReplicas != int32(replicas) {
		return false
	}
	if deployment.Status.ReadyReplicas != int32(replicas) {
		return false
	}
	if deployment.Status.UnavailableReplicas >= 1 {
		return false
	}
	if deployment.Status.ObservedGeneration <= 1 {
		return false
	}

	return true
}

// waitForDeploymentDelete watches for a deployment delete event.
func (r *CheckRunner) waitForDeploymentDelete(ctx context.Context) error {
	// Start a watch for deletion events.
	watcher, err := r.client.AppsV1().Deployments(r.cfg.CheckNamespace).Watch(ctx, metav1.ListOptions{
		Watch:         true,
		FieldSelector: "metadata.name=" + r.cfg.CheckDeploymentName,
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	// Consume watch events until deleted.
	for event := range watcher.ResultChan() {
		deployment, ok := event.Object.(*appsv1.Deployment)
		if !ok {
			log.Infoln("Got a watch event for a non-deployment object -- ignoring.")
			continue
		}
		log.Debugln("Received an event watching for deployment changes:", deployment.Name, "got event", event.Type)
		if event.Type == watch.Deleted {
			log.Infoln("Received", event.Type, "while watching for deployment", deployment.Name, "to be deleted")
			return nil
		}
	}

	return fmt.Errorf("deployment watch channel closed without delete event")
}
