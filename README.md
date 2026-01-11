# Kuberhealthy Deployment Check

This check validates that Kubernetes can create a Deployment and Service, reach the Service over HTTP, and clean everything up afterward. It is designed for Kuberhealthy v3 `HealthCheck` resources.

## What It Does

1. Cleans up any leftover Deployment/Service from prior runs.
2. Creates a Deployment with the configured replica count.
3. Creates a ClusterIP Service targeting the Deployment.
4. Sends HTTP GET requests to the Service until it receives `200 OK`.
5. Optionally performs a rolling update and validates the Service again.
6. Deletes the Deployment and Service.

## Configuration

All configuration is controlled via environment variables.

- `CHECK_IMAGE`: Initial container image. Default `nginxinc/nginx-unprivileged:1.17.8`.
- `CHECK_IMAGE_ROLL_TO`: Container image to roll to. Default `nginxinc/nginx-unprivileged:1.17.9`.
- `CHECK_IMAGE_PULL_SECRET`: Name of Image Pull Secret to use for above images.
- `CHECK_DEPLOYMENT_NAME`: Name for the check deployment. Default `deployment-deployment`.
- `CHECK_SERVICE_NAME`: Name for the check service. Default `deployment-svc`.
- `CHECK_NAMESPACE`: Namespace for the check resources. Default `kuberhealthy`. The pod namespace file is read first.
- `CHECK_DEPLOYMENT_REPLICAS`: Number of replicas. Default `2`.
- `CHECK_DEPLOYMENT_ROLLING_UPDATE`: Boolean to enable rolling update. Default `false`.
- `CHECK_CONTAINER_PORT`: Container port. Default `8080`.
- `CHECK_POD_CPU_REQUEST`: CPU request in millicores. Default `15`.
- `CHECK_POD_CPU_LIMIT`: CPU limit in millicores. Default `75`.
- `CHECK_POD_MEM_REQUEST`: Memory request in Mi. Default `20`.
- `CHECK_POD_MEM_LIMIT`: Memory limit in Mi. Default `75`.
- `NODE_SELECTOR`: Comma-separated `key=value` pairs applied to the deployment.
- `TOLERATIONS`: Comma-separated tolerations in `key=value:effect` format.
- `ADDITIONAL_ENV_VARS`: Comma-separated `key=value` pairs passed into the deployment container.
- `SHUTDOWN_GRACE_PERIOD`: Shutdown grace period (duration string). Default `30s`.
- `DEBUG`: Enable debug logging.

Kuberhealthy injects these variables automatically into the check pod:

- `KH_REPORTING_URL`
- `KH_RUN_UUID`
- `KH_CHECK_RUN_DEADLINE`

## Build

Use the `Justfile` to build or test the check:

```bash
just build
just test
```

## Example HealthCheck

This example creates 4 replicas and performs a rolling update:

```yaml
apiVersion: kuberhealthy.github.io/v2
kind: HealthCheck
metadata:
  name: deployment
  namespace: kuberhealthy
spec:
  runInterval: 10m
  timeout: 15m
  podSpec:
    spec:
      containers:
        - name: deployment
          image: kuberhealthy/deployment-check:latest
          env:
            - name: CHECK_DEPLOYMENT_REPLICAS
              value: "4"
            - name: CHECK_DEPLOYMENT_ROLLING_UPDATE
              value: "true"
          resources:
            requests:
              cpu: 25m
              memory: 15Mi
            limits:
              cpu: "1"
      restartPolicy: Never
      serviceAccountName: deployment-sa
      terminationGracePeriodSeconds: 60
```

A full install bundle with RBAC is available in `healthcheck.yaml`.

## RBAC

The check creates and deletes Deployments and Services, and it reads Pods and Events for diagnostics. The `healthcheck.yaml` file includes a Role and RoleBinding with those permissions.
