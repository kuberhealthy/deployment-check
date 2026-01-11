package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kuberhealthy/kuberhealthy/v3/pkg/checkclient"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// defaultCheckImageURL sets the initial deployment container image.
	defaultCheckImageURL = "nginxinc/nginx-unprivileged:1.17.8"
	// defaultCheckImageURLB sets the rolling update image when enabled.
	defaultCheckImageURLB = "nginxinc/nginx-unprivileged:1.17.9"

	// defaultCheckContainerPort sets the container port exposed by the deployment.
	defaultCheckContainerPort = int32(8080)
	// defaultCheckLoadBalancerPort sets the service port to hit inside the cluster.
	defaultCheckLoadBalancerPort = int32(80)

	// defaultCheckDeploymentName is the name for the test deployment.
	defaultCheckDeploymentName = "deployment-deployment"
	// defaultCheckServiceName is the name for the test service.
	defaultCheckServiceName = "deployment-svc"
	// defaultCheckServiceAccount is the service account to run with.
	defaultCheckServiceAccount = "default"
	// defaultCheckNamespace is the default namespace to run in.
	defaultCheckNamespace = "kuberhealthy"

	// defaultCheckDeploymentReplicas sets the default replica count.
	defaultCheckDeploymentReplicas = 2

	// defaultCheckTimeLimit sets the fallback check duration.
	defaultCheckTimeLimit = time.Minute * 15
	// defaultShutdownGracePeriod sets the fallback shutdown grace period.
	defaultShutdownGracePeriod = time.Second * 30

	// defaultMillicoreRequest is the default CPU request in millicores.
	defaultMillicoreRequest = 15
	// defaultMillicoreLimit is the default CPU limit in millicores.
	defaultMillicoreLimit = 75
	// defaultMemoryRequest is the default memory request in bytes (20Mi).
	defaultMemoryRequest = 20 * 1024 * 1024
	// defaultMemoryLimit is the default memory limit in bytes (75Mi).
	defaultMemoryLimit = 75 * 1024 * 1024
)

// CheckConfig describes the deployment check configuration.
type CheckConfig struct {
	// Debug enables verbose logging for the check.
	Debug bool
	// KubeConfigPath points to the kubeconfig for out-of-cluster runs.
	KubeConfigPath string
	// CheckImageURL is the initial image for the test deployment.
	CheckImageURL string
	// CheckImageURLRollTo is the image used for rolling updates.
	CheckImageURLRollTo string
	// CheckImagePullSecret is the optional image pull secret name.
	CheckImagePullSecret string
	// CheckDeploymentName is the deployment name.
	CheckDeploymentName string
	// CheckServiceName is the service name.
	CheckServiceName string
	// CheckContainerPort is the container port for HTTP.
	CheckContainerPort int32
	// CheckLoadBalancerPort is the service port for HTTP.
	CheckLoadBalancerPort int32
	// CheckNamespace is the namespace for the check.
	CheckNamespace string
	// CheckDeploymentReplicas is the number of deployment replicas.
	CheckDeploymentReplicas int
	// CheckDeploymentTolerations are pod tolerations to apply.
	CheckDeploymentTolerations []corev1.Toleration
	// CheckDeploymentNodeSelectors are node selector labels to apply.
	CheckDeploymentNodeSelectors map[string]string
	// CheckServiceAccount is the service account name to use.
	CheckServiceAccount string
	// MillicoreRequest is the CPU request in millicores.
	MillicoreRequest int
	// MillicoreLimit is the CPU limit in millicores.
	MillicoreLimit int
	// MemoryRequest is the memory request in bytes.
	MemoryRequest int
	// MemoryLimit is the memory limit in bytes.
	MemoryLimit int
	// CheckTimeLimit is the time budget for the full check.
	CheckTimeLimit time.Duration
	// RollingUpdate enables the rolling update flow.
	RollingUpdate bool
	// AdditionalEnvVars are extra env vars passed to the deployment container.
	AdditionalEnvVars map[string]string
	// ShutdownGracePeriod is the time allowed for cleanup on termination.
	ShutdownGracePeriod time.Duration
}

// parseConfig reads environment variables into a CheckConfig for the check runtime.
func parseConfig() (*CheckConfig, error) {
	// Start with base defaults and placeholders.
	cfg := &CheckConfig{}

	// Set a default kubeconfig path for out-of-cluster use.
	cfg.KubeConfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")

	// Parse debug before anything else to enable verbose logging.
	debugEnv := os.Getenv("DEBUG")
	if len(debugEnv) != 0 {
		debugValue, err := strconv.ParseBool(debugEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DEBUG: %w", err)
		}
		cfg.Debug = debugValue
	}

	// Apply logging configuration.
	if cfg.Debug {
		log.Infoln("Debug logging enabled.")
		log.SetLevel(log.DebugLevel)
	}

	// Store base images.
	cfg.CheckImageURL = defaultCheckImageURL
	cfg.CheckImageURLRollTo = defaultCheckImageURLB

	// Override images if env vars are set.
	checkImageEnv := os.Getenv("CHECK_IMAGE")
	if len(checkImageEnv) != 0 {
		cfg.CheckImageURL = checkImageEnv
		log.Infoln("Parsed CHECK_IMAGE:", cfg.CheckImageURL)
	}
	checkImageRollEnv := os.Getenv("CHECK_IMAGE_ROLL_TO")
	if len(checkImageRollEnv) != 0 {
		cfg.CheckImageURLRollTo = checkImageRollEnv
		log.Infoln("Parsed CHECK_IMAGE_ROLL_TO:", cfg.CheckImageURLRollTo)
	}

	// Parse image pull secret.
	cfg.CheckImagePullSecret = os.Getenv("CHECK_IMAGE_PULL_SECRET")
	if len(cfg.CheckImagePullSecret) != 0 {
		log.Infoln("Parsed CHECK_IMAGE_PULL_SECRET:", cfg.CheckImagePullSecret)
	}

	// Parse deployment name.
	cfg.CheckDeploymentName = defaultCheckDeploymentName
	checkDeploymentNameEnv := os.Getenv("CHECK_DEPLOYMENT_NAME")
	if len(checkDeploymentNameEnv) != 0 {
		cfg.CheckDeploymentName = checkDeploymentNameEnv
		log.Infoln("Parsed CHECK_DEPLOYMENT_NAME:", cfg.CheckDeploymentName)
	}

	// Parse service name.
	cfg.CheckServiceName = defaultCheckServiceName
	checkServiceNameEnv := os.Getenv("CHECK_SERVICE_NAME")
	if len(checkServiceNameEnv) != 0 {
		cfg.CheckServiceName = checkServiceNameEnv
		log.Infoln("Parsed CHECK_SERVICE_NAME:", cfg.CheckServiceName)
	}

	// Parse container port.
	cfg.CheckContainerPort = defaultCheckContainerPort
	checkContainerPortEnv := os.Getenv("CHECK_CONTAINER_PORT")
	if len(checkContainerPortEnv) != 0 {
		portValue, err := strconv.Atoi(checkContainerPortEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_CONTAINER_PORT: %w", err)
		}
		cfg.CheckContainerPort = int32(portValue)
		log.Infoln("Parsed CHECK_CONTAINER_PORT:", cfg.CheckContainerPort)
	}

	// Parse service port.
	cfg.CheckLoadBalancerPort = defaultCheckLoadBalancerPort
	checkLoadBalancerPortEnv := os.Getenv("CHECK_LOAD_BALANCER_PORT")
	if len(checkLoadBalancerPortEnv) != 0 {
		portValue, err := strconv.Atoi(checkLoadBalancerPortEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_LOAD_BALANCER_PORT: %w", err)
		}
		cfg.CheckLoadBalancerPort = int32(portValue)
		log.Infoln("Parsed CHECK_LOAD_BALANCER_PORT:", cfg.CheckLoadBalancerPort)
	}

	// Parse namespace with service account fallback.
	cfg.CheckNamespace = defaultCheckNamespace
	namespaceBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		log.Warnln("Failed to read pod namespace file:", err.Error())
	}
	if len(namespaceBytes) != 0 {
		cfg.CheckNamespace = strings.TrimSpace(string(namespaceBytes))
		log.Infoln("Found pod namespace:", cfg.CheckNamespace)
	}
	checkNamespaceEnv := os.Getenv("CHECK_NAMESPACE")
	if len(checkNamespaceEnv) != 0 {
		cfg.CheckNamespace = checkNamespaceEnv
		log.Infoln("Parsed CHECK_NAMESPACE:", cfg.CheckNamespace)
	}
	log.Infoln("Performing check in", cfg.CheckNamespace, "namespace.")

	// Parse deployment replicas.
	cfg.CheckDeploymentReplicas = defaultCheckDeploymentReplicas
	checkDeploymentReplicasEnv := os.Getenv("CHECK_DEPLOYMENT_REPLICAS")
	if len(checkDeploymentReplicasEnv) != 0 {
		replicaValue, err := strconv.Atoi(checkDeploymentReplicasEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_DEPLOYMENT_REPLICAS: %w", err)
		}
		if replicaValue < 1 {
			return nil, fmt.Errorf("CHECK_DEPLOYMENT_REPLICAS must be >= 1, got %d", replicaValue)
		}
		cfg.CheckDeploymentReplicas = replicaValue
		log.Infoln("Parsed CHECK_DEPLOYMENT_REPLICAS:", cfg.CheckDeploymentReplicas)
	}

	// Parse tolerations for the deployment.
	cfg.CheckDeploymentTolerations = make([]corev1.Toleration, 0)
	checkDeploymentTolerationsEnv := os.Getenv("TOLERATIONS")
	if len(checkDeploymentTolerationsEnv) != 0 {
		tolerations, err := parseTolerations(checkDeploymentTolerationsEnv)
		if err != nil {
			return nil, err
		}
		cfg.CheckDeploymentTolerations = tolerations
		log.Infoln("Parsed TOLERATIONS:", cfg.CheckDeploymentTolerations)
	}

	// Parse node selectors for the deployment.
	cfg.CheckDeploymentNodeSelectors = make(map[string]string)
	checkDeploymentNodeSelectorsEnv := os.Getenv("NODE_SELECTOR")
	if len(checkDeploymentNodeSelectorsEnv) != 0 {
		selectors, err := parseNodeSelectors(checkDeploymentNodeSelectorsEnv)
		if err != nil {
			return nil, err
		}
		cfg.CheckDeploymentNodeSelectors = selectors
		log.Infoln("Parsed NODE_SELECTOR:", cfg.CheckDeploymentNodeSelectors)
	}

	// Parse resource requests and limits.
	cfg.MillicoreRequest = defaultMillicoreRequest
	millicoreRequestEnv := os.Getenv("CHECK_POD_CPU_REQUEST")
	if len(millicoreRequestEnv) != 0 {
		cpuValue, err := strconv.ParseInt(millicoreRequestEnv, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_POD_CPU_REQUEST: %w", err)
		}
		cfg.MillicoreRequest = int(cpuValue)
		log.Infoln("Parsed CHECK_POD_CPU_REQUEST:", cfg.MillicoreRequest)
	}

	cfg.MillicoreLimit = defaultMillicoreLimit
	millicoreLimitEnv := os.Getenv("CHECK_POD_CPU_LIMIT")
	if len(millicoreLimitEnv) != 0 {
		cpuValue, err := strconv.ParseInt(millicoreLimitEnv, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_POD_CPU_LIMIT: %w", err)
		}
		cfg.MillicoreLimit = int(cpuValue)
		log.Infoln("Parsed CHECK_POD_CPU_LIMIT:", cfg.MillicoreLimit)
	}

	cfg.MemoryRequest = defaultMemoryRequest
	memoryRequestEnv := os.Getenv("CHECK_POD_MEM_REQUEST")
	if len(memoryRequestEnv) != 0 {
		memValue, err := strconv.ParseInt(memoryRequestEnv, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_POD_MEM_REQUEST: %w", err)
		}
		cfg.MemoryRequest = int(memValue) * 1024 * 1024
		log.Infoln("Parsed CHECK_POD_MEM_REQUEST:", cfg.MemoryRequest)
	}

	cfg.MemoryLimit = defaultMemoryLimit
	memoryLimitEnv := os.Getenv("CHECK_POD_MEM_LIMIT")
	if len(memoryLimitEnv) != 0 {
		memValue, err := strconv.ParseInt(memoryLimitEnv, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_POD_MEM_LIMIT: %w", err)
		}
		cfg.MemoryLimit = int(memValue) * 1024 * 1024
		log.Infoln("Parsed CHECK_POD_MEM_LIMIT:", cfg.MemoryLimit)
	}

	// Parse service account name.
	cfg.CheckServiceAccount = defaultCheckServiceAccount
	checkServiceAccountEnv := os.Getenv("CHECK_SERVICE_ACCOUNT")
	if len(checkServiceAccountEnv) != 0 {
		cfg.CheckServiceAccount = checkServiceAccountEnv
		log.Infoln("Parsed CHECK_SERVICE_ACCOUNT:", cfg.CheckServiceAccount)
	}

	// Parse check deadline from injected env.
	cfg.CheckTimeLimit = defaultCheckTimeLimit
	deadlineTime, err := checkclient.GetDeadline()
	if err != nil {
		log.Infoln("There was an issue getting the check deadline:", err.Error())
	}
	if err == nil {
		cfg.CheckTimeLimit = deadlineTime.Sub(time.Now().Add(time.Second * 5))
	}
	log.Infoln("Check time limit set to:", cfg.CheckTimeLimit)

	// Parse rolling update setting.
	rollingUpdateEnv := os.Getenv("CHECK_DEPLOYMENT_ROLLING_UPDATE")
	if len(rollingUpdateEnv) != 0 {
		rollingValue, err := strconv.ParseBool(rollingUpdateEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CHECK_DEPLOYMENT_ROLLING_UPDATE: %w", err)
		}
		cfg.RollingUpdate = rollingValue
	}
	log.Infoln("Parsed CHECK_DEPLOYMENT_ROLLING_UPDATE:", cfg.RollingUpdate)
	if cfg.RollingUpdate {
		if cfg.CheckImageURL == cfg.CheckImageURLRollTo {
			log.Infoln("The same container image cannot be used for the rolling-update check. Using defaults.")
			cfg.CheckImageURL = defaultCheckImageURL
			cfg.CheckImageURLRollTo = defaultCheckImageURLB
			log.Infoln("Setting initial container image to:", cfg.CheckImageURL)
			log.Infoln("Setting update container image to:", cfg.CheckImageURLRollTo)
		}
		log.Infoln("Check deployment image will be rolled from [" + cfg.CheckImageURL + "] to [" + cfg.CheckImageURLRollTo + "]")
	}

	// Parse additional env vars for the deployment.
	cfg.AdditionalEnvVars = make(map[string]string)
	additionalEnvVarsEnv := os.Getenv("ADDITIONAL_ENV_VARS")
	if len(additionalEnvVarsEnv) != 0 {
		additionalVars, err := parseAdditionalEnvVars(additionalEnvVarsEnv)
		if err != nil {
			return nil, err
		}
		cfg.AdditionalEnvVars = additionalVars
		log.Infoln("Parsed ADDITIONAL_ENV_VARS:", cfg.AdditionalEnvVars)
	}

	// Parse shutdown grace period.
	cfg.ShutdownGracePeriod = defaultShutdownGracePeriod
	shutdownGracePeriodEnv := os.Getenv("SHUTDOWN_GRACE_PERIOD")
	if len(shutdownGracePeriodEnv) != 0 {
		durationValue, err := time.ParseDuration(shutdownGracePeriodEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SHUTDOWN_GRACE_PERIOD: %w", err)
		}
		if durationValue.Seconds() < 1 {
			return nil, fmt.Errorf("SHUTDOWN_GRACE_PERIOD must be >= 1s, got %.0f", durationValue.Seconds())
		}
		cfg.ShutdownGracePeriod = durationValue
		log.Infoln("Parsed SHUTDOWN_GRACE_PERIOD:", cfg.ShutdownGracePeriod)
	}

	// Ensure logrus and checkclient share debug state.
	checkclient.Debug = cfg.Debug

	return cfg, nil
}

// parseTolerations converts a comma-separated tolerations string into objects for the pod spec.
func parseTolerations(raw string) ([]corev1.Toleration, error) {
	// Split entries on commas for key/value pairs.
	entries := strings.Split(raw, ",")
	if len(entries) == 0 {
		return nil, fmt.Errorf("no tolerations provided")
	}

	// Build the tolerations slice.
	tolerations := make([]corev1.Toleration, 0)
	for _, entry := range entries {
		// Split key and value/effect parts.
		parts := strings.Split(entry, "=")
		if len(parts) != 2 {
			log.Warnln("Unable to parse key value pair:", entry)
			log.Warnln("Setting operator to", corev1.TolerationOpExists)
			toleration := corev1.Toleration{
				Key:      parts[0],
				Operator: corev1.TolerationOpExists,
			}
			log.Infoln("Adding toleration to deployment:", toleration)
			tolerations = append(tolerations, toleration)
			continue
		}

		// Split value/effect when present.
		valueEffect := strings.Split(parts[1], ":")
		if len(valueEffect) != 2 {
			log.Warnln("Unable to parse complete toleration value and effect:", valueEffect)
			toleration := corev1.Toleration{
				Key:      parts[0],
				Operator: corev1.TolerationOpEqual,
				Value:    parts[1],
			}
			log.Infoln("Adding toleration to deployment:", toleration)
			tolerations = append(tolerations, toleration)
			continue
		}

		// Build a full toleration when both value and effect are present.
		toleration := corev1.Toleration{
			Key:      parts[0],
			Operator: corev1.TolerationOpEqual,
			Value:    valueEffect[0],
			Effect:   corev1.TaintEffect(valueEffect[1]),
		}
		log.Infoln("Adding toleration to deployment:", toleration)
		tolerations = append(tolerations, toleration)
	}

	return tolerations, nil
}

// parseNodeSelectors converts a comma-separated selector string into a map for the pod spec.
func parseNodeSelectors(raw string) (map[string]string, error) {
	// Split entries into key/value pairs.
	entries := strings.Split(raw, ",")
	if len(entries) == 0 {
		return nil, fmt.Errorf("no node selectors provided")
	}

	// Build the selector map.
	selectors := make(map[string]string)
	for _, entry := range entries {
		parts := strings.Split(entry, "=")
		if len(parts) != 2 {
			log.Warnln("Unable to parse key value pair:", entry)
			continue
		}
		_, exists := selectors[parts[0]]
		if exists {
			continue
		}
		selectors[parts[0]] = parts[1]
	}

	return selectors, nil
}

// parseAdditionalEnvVars parses key=value pairs into a map for container env vars.
func parseAdditionalEnvVars(raw string) (map[string]string, error) {
	// Split entries into key/value pairs.
	entries := strings.Split(raw, ",")
	if len(entries) == 0 {
		return nil, fmt.Errorf("no additional env vars provided")
	}

	// Build the env var map.
	vars := make(map[string]string)
	for _, entry := range entries {
		parts := strings.Split(entry, "=")
		if len(parts) != 2 {
			log.Warnln("Unable to parse key value pair:", entry)
			continue
		}
		_, exists := vars[parts[0]]
		if exists {
			continue
		}
		vars[parts[0]] = parts[1]
	}

	return vars, nil
}
