package main

import "testing"

// TestBuildServiceURLRejectsBlankAddress ensures blank addresses fail early.
func TestBuildServiceURLRejectsBlankAddress(t *testing.T) {
	// Build the URL from an empty address.
	_, err := buildServiceURL("")
	if err == nil {
		t.Fatal("expected an error for a blank service address")
	}
}

// TestBuildServiceURLPrefixesIPv4Address ensures IPv4 addresses are prefixed with HTTP.
func TestBuildServiceURLPrefixesIPv4Address(t *testing.T) {
	// Build the URL from an IPv4 service address.
	url, err := buildServiceURL("10.0.0.15")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if url != "http://10.0.0.15" {
		t.Fatalf("expected http://10.0.0.15, got %s", url)
	}
}

// TestBuildServiceURLBracketsIPv6Address ensures IPv6 service IPs become valid URLs.
func TestBuildServiceURLBracketsIPv6Address(t *testing.T) {
	// Build the URL from an IPv6 service address.
	url, err := buildServiceURL("fd00:4:32::7f86")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if url != "http://[fd00:4:32::7f86]" {
		t.Fatalf("expected http://[fd00:4:32::7f86], got %s", url)
	}
}
