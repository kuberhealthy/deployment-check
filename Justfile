IMAGE := "kuberhealthy/deployment-check"
TAG := "latest"

# Build the deployment check container locally.
build:
	podman build -f Containerfile -t {{IMAGE}}:{{TAG}} .

# Run the unit tests for the deployment check.
test:
	go test ./...

# Build the deployment check binary locally.
binary:
	go build -o bin/deployment-check ./
