package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kuberhealthy/kuberhealthy/v3/pkg/checkclient"
	log "github.com/sirupsen/logrus"
)

// main initializes configuration, dependencies, and executes the deployment check.
func main() {
	// Parse configuration from environment variables.
	cfg, err := parseConfig()
	if err != nil {
		log.Fatalln("Failed to parse config:", err.Error())
	}

	// Build a Kubernetes clientset for API access.
	clientset, err := createKubeClient(cfg.KubeConfigPath)
	if err != nil {
		reportFailure([]string{"failed to create a kubernetes client: " + err.Error()})
		return
	}
	log.Infoln("Kubernetes client created.")

	// Create a context that enforces the check deadline.
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CheckTimeLimit)
	defer cancel()

	// Build the runner that will execute the check.
	runner := newCheckRunner(cfg, clientset, now)

	// Start interrupt handling in the background.
	interrupts := make(chan os.Signal, 3)
	signal.Notify(interrupts, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGINT)
	go runner.handleInterrupts(ctx, cancel, interrupts)

	// Run the check and report status.
	err = runner.run(ctx)
	if err != nil {
		reportFailure([]string{err.Error()})
		return
	}

	reportSuccess()
}

// handleInterrupts listens for signals and performs cleanup before exit.
func (r *CheckRunner) handleInterrupts(ctx context.Context, cancel context.CancelFunc, interrupts chan os.Signal) {
	// Wait for the first interrupt signal.
	sig := <-interrupts
	log.Infoln("Received an interrupt signal from the signal channel.")
	log.Debugln("Signal received was:", sig.String())

	// Cancel the main context to halt ongoing work.
	log.Debugln("Cancelling context.")
	cancel()

	// Attempt cleanup with a grace period.
	log.Infoln("Shutting down.")

	cleanupChan := make(chan error, 1)
	go r.runCleanupAsync(ctx, cleanupChan)

	select {
	case sig = <-interrupts:
		log.Warnln("Received a second interrupt signal from the signal channel.")
		log.Debugln("Signal received was:", sig.String())
	case cleanupErr := <-cleanupChan:
		log.Infoln("Received a complete signal, clean up completed.")
		if cleanupErr != nil {
			log.Errorln("Failed to clean up check resources properly:", cleanupErr.Error())
		}
	case <-time.After(r.cfg.ShutdownGracePeriod):
		log.Infoln("Clean up took too long to complete and timed out.")
	}

	os.Exit(0)
}

// reportFailure sends a failure report to Kuberhealthy.
func reportFailure(errors []string) {
	// Log and send the failure report.
	log.Errorln("Reporting errors to Kuberhealthy:", errors)
	err := checkclient.ReportFailure(errors)
	if err != nil {
		log.Fatalln("Error reporting to Kuberhealthy:", err.Error())
	}
}

// reportSuccess sends a success report to Kuberhealthy.
func reportSuccess() {
	// Log and send the success report.
	log.Infoln("Reporting success to Kuberhealthy.")
	err := checkclient.ReportSuccess()
	if err != nil {
		log.Fatalln("Error reporting to Kuberhealthy:", err.Error())
	}
}
