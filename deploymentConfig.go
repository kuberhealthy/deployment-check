package main

import (
	"errors"
	"math"
	"strconv"

	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// deploymentLabelKey marks resources created by the check.
	deploymentLabelKey = "deployment-timestamp"
	// deploymentLabelValueBase is combined with the run timestamp.
	deploymentLabelValueBase = "unix-"
	// deploymentMinReadySeconds sets the minimum ready time for replicas.
	deploymentMinReadySeconds = 5

	// deploymentMaxSurgeDefault is a fallback for max surge.
	deploymentMaxSurgeDefault = 2
	// deploymentMaxUnavailableDefault is a fallback for max unavailable.
	deploymentMaxUnavailableDefault = 2

	// deploymentImagePullPolicy sets a sane default for the check image.
	deploymentImagePullPolicy = "IfNotPresent"

	// probeFailureThreshold sets readiness and liveness thresholds.
	probeFailureThreshold = 5
	// probeSuccessThreshold sets readiness and liveness thresholds.
	probeSuccessThreshold = 1
	// probeInitialDelaySeconds controls probe startup delay.
	probeInitialDelaySeconds = 2
	// probeTimeoutSeconds controls probe timeouts.
	probeTimeoutSeconds = 2
	// probePeriodSeconds controls probe check cadence.
	probePeriodSeconds = 15
)

// createDeploymentConfig builds a deployment manifest for the check image.
func (r *CheckRunner) createDeploymentConfig(image string) *appsv1.Deployment {
	// Allocate a deployment object to populate.
	deployment := &appsv1.Deployment{}

	// Use the specified image for this deployment.
	checkImage := image
	log.Infoln("Creating deployment resource with", r.cfg.CheckDeploymentReplicas, "replica(s) in", r.cfg.CheckNamespace, "namespace using image ["+checkImage+"] with environment variables:", r.cfg.AdditionalEnvVars)

	// Validate the image setting to avoid empty manifests.
	if len(checkImage) == 0 {
		err := errors.New("check image url for container is empty: " + checkImage)
		log.Warnln(err.Error())
		return deployment
	}

	// Build the container spec for the deployment.
	container := r.createContainerConfig(checkImage)
	containers := []corev1.Container{container}

	// Ensure node selector map is nil when empty.
	nodeSelectors := r.cfg.CheckDeploymentNodeSelectors
	if len(nodeSelectors) == 0 {
		nodeSelectors = nil
	}

	// Configure a short pod termination grace period.
	graceSeconds := int64(1)

	// Assemble the pod spec for the deployment.
	podSpec := corev1.PodSpec{
		Containers:                    containers,
		NodeSelector:                  nodeSelectors,
		RestartPolicy:                 corev1.RestartPolicyAlways,
		TerminationGracePeriodSeconds: &graceSeconds,
		ServiceAccountName:            r.cfg.CheckServiceAccount,
		Tolerations:                   r.cfg.CheckDeploymentTolerations,
	}

	// Attach image pull secrets if configured.
	if len(r.cfg.CheckImagePullSecret) != 0 {
		secrets := []corev1.LocalObjectReference{{Name: r.cfg.CheckImagePullSecret}}
		podSpec.ImagePullSecrets = secrets
	}

	// Build labels for the deployment and pod template.
	labels := make(map[string]string)
	labels[deploymentLabelKey] = deploymentLabelValueBase + strconv.Itoa(int(r.now.Unix()))
	labels["source"] = "kuberhealthy"

	// Assemble the pod template.
	podTemplateSpec := corev1.PodTemplateSpec{
		Spec: podSpec,
	}
	podTemplateSpec.ObjectMeta.Labels = labels
	podTemplateSpec.ObjectMeta.Name = r.cfg.CheckDeploymentName
	podTemplateSpec.ObjectMeta.Namespace = r.cfg.CheckNamespace

	// Build the selector from the labels.
	labelSelector := metav1.LabelSelector{
		MatchLabels: labels,
	}

	// Calculate rolling update values based on replica count.
	maxSurge := math.Ceil(float64(r.cfg.CheckDeploymentReplicas) / float64(2))
	maxUnavailable := math.Ceil(float64(r.cfg.CheckDeploymentReplicas) / float64(2))
	if maxSurge < 1 {
		maxSurge = deploymentMaxSurgeDefault
	}
	if maxUnavailable < 1 {
		maxUnavailable = deploymentMaxUnavailableDefault
	}

	// Build the rolling update strategy.
	rollingUpdateSpec := appsv1.RollingUpdateDeployment{
		MaxUnavailable: &intstr.IntOrString{
			IntVal: int32(maxUnavailable),
			StrVal: strconv.Itoa(int(maxUnavailable)),
		},
		MaxSurge: &intstr.IntOrString{
			IntVal: int32(maxSurge),
			StrVal: strconv.Itoa(int(maxSurge)),
		},
	}
	deployStrategy := appsv1.DeploymentStrategy{
		Type:          appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &rollingUpdateSpec,
	}

	// Build the deployment spec.
	replicas := int32(r.cfg.CheckDeploymentReplicas)
	deploySpec := appsv1.DeploymentSpec{
		Strategy:        deployStrategy,
		MinReadySeconds: deploymentMinReadySeconds,
		Replicas:        &replicas,
		Selector:        &labelSelector,
		Template:        podTemplateSpec,
	}

	// Populate the deployment metadata and spec.
	deployment.ObjectMeta.Name = r.cfg.CheckDeploymentName
	deployment.ObjectMeta.Namespace = r.cfg.CheckNamespace
	deployment.Spec = deploySpec

	return deployment
}

// createContainerConfig builds the main container spec for the deployment.
func (r *CheckRunner) createContainerConfig(imageURL string) corev1.Container {
	// Emit configuration details to the logs.
	log.Infoln("Creating container using image ["+imageURL+"] with environment variables:", r.cfg.AdditionalEnvVars)

	// Configure the container port.
	basicPort := corev1.ContainerPort{
		ContainerPort: r.cfg.CheckContainerPort,
	}
	containerPorts := []corev1.ContainerPort{basicPort}

	// Build resource requests.
	requests := make(map[corev1.ResourceName]resource.Quantity)
	requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(r.cfg.MillicoreRequest), resource.DecimalSI)
	requests[corev1.ResourceMemory] = *resource.NewQuantity(int64(r.cfg.MemoryRequest), resource.BinarySI)

	// Build resource limits.
	limits := make(map[corev1.ResourceName]resource.Quantity)
	limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(r.cfg.MillicoreLimit), resource.DecimalSI)
	limits[corev1.ResourceMemory] = *resource.NewQuantity(int64(r.cfg.MemoryLimit), resource.BinarySI)

	// Assemble resource requirements.
	resources := corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}

	// Build environment variable list from config.
	envs := make([]corev1.EnvVar, 0)
	for key, value := range r.cfg.AdditionalEnvVars {
		envVar := corev1.EnvVar{
			Name:  key,
			Value: value,
		}
		envs = append(envs, envVar)
	}

	// Assemble the liveness probe.
	liveProbe := corev1.Probe{
		InitialDelaySeconds: probeInitialDelaySeconds,
		TimeoutSeconds:      probeTimeoutSeconds,
		PeriodSeconds:       probePeriodSeconds,
		SuccessThreshold:    probeSuccessThreshold,
		FailureThreshold:    probeFailureThreshold,
	}

	// Assemble the readiness probe.
	readyProbe := corev1.Probe{
		InitialDelaySeconds: probeInitialDelaySeconds,
		TimeoutSeconds:      probeTimeoutSeconds,
		PeriodSeconds:       probePeriodSeconds,
		SuccessThreshold:    probeSuccessThreshold,
		FailureThreshold:    probeFailureThreshold,
	}

	// Build the container spec.
	container := corev1.Container{
		Name:            "deployment-container",
		Image:           imageURL,
		ImagePullPolicy: deploymentImagePullPolicy,
		Ports:           containerPorts,
		Resources:       resources,
		Env:             envs,
		LivenessProbe:   &liveProbe,
		ReadinessProbe:  &readyProbe,
	}

	return container
}
