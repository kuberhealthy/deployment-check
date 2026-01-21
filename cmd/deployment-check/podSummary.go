package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// decorateDeploymentError adds deployment stage and pod context to an error.
func (r *CheckRunner) decorateDeploymentError(ctx context.Context, stage string, err error) error {
	// Guard against nil errors.
	if err == nil {
		return nil
	}

	// Capture the deployment pod snapshot for troubleshooting.
	podSummary := r.deploymentPodSummary(ctx)
	return fmt.Errorf("%s failed: %w; pod status: %s", stage, err, podSummary)
}

// deploymentPodSummary lists pods for the current run and summarizes their state.
func (r *CheckRunner) deploymentPodSummary(ctx context.Context) string {
	// Bound the pod lookup to a short timeout.
	summaryCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Use the current run timestamp label to locate pods.
	labelSelector := deploymentLabelKey + "=" + deploymentLabelValueBase + fmt.Sprint(r.now.Unix())
	podList, err := r.client.CoreV1().Pods(r.cfg.CheckNamespace).List(summaryCtx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "failed to list deployment pods: " + err.Error()
	}

	// Return a short message when no pods are found.
	if len(podList.Items) == 0 {
		return "no deployment pods found"
	}

	// Build per-pod summaries.
	summaries := make([]string, 0, len(podList.Items))
	for _, pod := range podList.Items {
		summaries = append(summaries, formatDeploymentPodSummary(pod))
	}

	return strings.Join(summaries, "; ")
}

// formatDeploymentPodSummary builds a single-line summary for a pod.
func formatDeploymentPodSummary(pod corev1.Pod) string {
	// Default the node name when scheduling has not happened yet.
	nodeName := pod.Spec.NodeName
	if len(nodeName) == 0 {
		nodeName = "unscheduled"
	}

	// Track readiness per container.
	readyCount := 0
	totalCount := len(pod.Status.ContainerStatuses)
	containerStates := make([]string, 0, totalCount)
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			readyCount++
		}
		state := describeContainerState(status)
		containerStates = append(containerStates, status.Name+"="+state)
	}

	// Fall back when there are no container statuses yet.
	if len(containerStates) == 0 {
		containerStates = append(containerStates, "none")
	}

	// Include pod-level reason when present.
	reason := pod.Status.Reason
	if len(reason) == 0 {
		reason = "none"
	}

	return fmt.Sprintf(
		"pod=%s node=%s phase=%s reason=%s ready=%d/%d containers=[%s]",
		pod.Name,
		nodeName,
		pod.Status.Phase,
		reason,
		readyCount,
		totalCount,
		strings.Join(containerStates, ","),
	)
}

// describeContainerState renders a container state in a compact form.
func describeContainerState(status corev1.ContainerStatus) string {
	// Prefer terminated states when present.
	if status.State.Terminated != nil {
		reason := status.State.Terminated.Reason
		if len(reason) == 0 {
			reason = "terminated"
		}
		return "terminated:" + reason
	}

	// Prefer waiting states over running.
	if status.State.Waiting != nil {
		reason := status.State.Waiting.Reason
		if len(reason) == 0 {
			reason = "waiting"
		}
		return "waiting:" + reason
	}

	// Default to running when no other state is set.
	if status.State.Running != nil {
		return "running"
	}

	return "unknown"
}
