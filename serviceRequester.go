package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// requestBackoffTimeout caps the HTTP retry window.
	requestBackoffTimeout = time.Minute * 3
	// requestBackoffMaxRetries caps the number of HTTP attempts.
	requestBackoffMaxRetries = 10
)

// requestServiceEndpoint performs a GET against the service endpoint with retries.
func (r *CheckRunner) requestServiceEndpoint(ctx context.Context, address string) error {
	// Validate address before attempting the request.
	if len(address) == 0 {
		return fmt.Errorf("given blank service address for HTTP call")
	}

	// Ensure the address is an HTTP URL.
	if !strings.HasPrefix(address, "http://") {
		address = "http://" + address
	}

	// Log the request intent.
	log.Infoln("Looking for a response from the endpoint.")
	log.Debugln("Setting timeout for backoff loop to:", requestBackoffTimeout)

	// Bound the backoff loop by time.
	deadline := time.Now().Add(requestBackoffTimeout)
	attempt := 1

	for {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			cleanupErr := r.cleanup(ctx)
			if cleanupErr != nil {
				return fmt.Errorf("context expired while waiting for %d from %s and cleanup failed: %w", http.StatusOK, address, cleanupErr)
			}
			return fmt.Errorf("context expired while waiting for %d from %s", http.StatusOK, address)
		default:
		}

		// Exit on timeout.
		if time.Now().After(deadline) {
			cleanupErr := r.cleanup(ctx)
			if cleanupErr != nil {
				return fmt.Errorf("backoff loop timed out and cleanup failed: %w", cleanupErr)
			}
			return fmt.Errorf("backoff loop for a %d response took too long and timed out", http.StatusOK)
		}

		// Stop after max retries.
		if attempt > requestBackoffMaxRetries {
			return fmt.Errorf("could not successfully make an HTTP request after %d attempts", attempt-1)
		}

		// Perform the request.
		log.Debugln("Making", http.MethodGet, "to", address)
		response, err := http.Get(address)
		if err == nil && response != nil {
			statusCode := response.StatusCode
			log.Debugln("Got a", statusCode)
			if statusCode == http.StatusOK {
				closeErr := response.Body.Close()
				if closeErr != nil {
					log.Debugln("Failed to close response body:", closeErr.Error())
				}
				log.Infoln("Successfully made an HTTP request on attempt:", attempt)
				log.Infoln("Got a", statusCode, "with a", http.MethodGet, "to", address)
				return nil
			}

			// Handle 502 from service mesh with nil error.
			if statusCode == http.StatusBadGateway {
				err = errors.New("received 502 from service endpoint")
			}

			closeErr := response.Body.Close()
			if closeErr != nil {
				log.Debugln("Failed to close response body:", closeErr.Error())
			}
		}

		// Log errors except for DNS delays.
		if err != nil {
			if !strings.Contains(err.Error(), "no such host") {
				log.Debugln("An error occurred making a", http.MethodGet, "request:", err)
			}
		}

		// Sleep with backoff before retrying.
		retrySleepSeconds := attempt * 5
		log.Infoln("Retrying in", retrySleepSeconds, "seconds.")
		time.Sleep(time.Duration(retrySleepSeconds) * time.Second)
		attempt++
	}
}
