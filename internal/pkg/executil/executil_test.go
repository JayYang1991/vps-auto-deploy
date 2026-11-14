package executil

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidatePort(t *testing.T) {
	validPorts := []int{1, 22, 80, 443, 20888, 65535}
	for _, p := range validPorts {
		if err := ValidatePort(p); err != nil {
			t.Errorf("expected port %d to be valid, got: %v", p, err)
		}
	}

	invalidPorts := []int{-1, 0, 65536, 99999}
	for _, p := range invalidPorts {
		if err := ValidatePort(p); err == nil {
			t.Errorf("expected port %d to be invalid, got nil error", p)
		}
	}
}

func TestValidateServiceName(t *testing.T) {
	validNames := []string{"vps-panel", "vps-panel.service", "sing-box", "cloudflared", "subconverter"}
	for _, name := range validNames {
		if err := ValidateServiceName(name); err != nil {
			t.Errorf("expected service %q to be valid, got: %v", name, err)
		}
	}

	invalidNames := []string{
		"vps-panel; rm -rf /",
		"service&evil",
		"name with spaces",
		"../../../etc/shadow",
		"service|cat",
	}
	for _, name := range invalidNames {
		if err := ValidateServiceName(name); err == nil {
			t.Errorf("expected service %q to be invalid, got nil error", name)
		}
	}
}

func TestShellInterpreterProhibited(t *testing.T) {
	shells := []string{"sh", "bash", "/bin/sh", "/bin/bash"}
	for _, shell := range shells {
		_, err := RunCommand(2*time.Second, shell, "-c", "echo hello")
		if err == nil || !strings.Contains(err.Error(), "security violation") {
			t.Errorf("expected security violation error for shell %q in RunCommand, got: %v", shell, err)
		}

		_, err = SafeRun(context.Background(), 2*time.Second, shell, "-c", "echo hello")
		if err == nil || !strings.Contains(err.Error(), "security violation") {
			t.Errorf("expected security violation error for shell %q in SafeRun, got: %v", shell, err)
		}
	}
}

func TestSafeRunAndRunCommand(t *testing.T) {
	out, err := SafeRun(context.Background(), 5*time.Second, "echo", "sec-test-ok")
	if err != nil {
		t.Fatalf("SafeRun failed: %v", err)
	}
	if !strings.Contains(string(out), "sec-test-ok") {
		t.Fatalf("SafeRun output mismatch: %s", string(out))
	}

	outStr, err := RunCommand(5*time.Second, "echo", "run-test-ok")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}
	if !strings.Contains(outStr, "run-test-ok") {
		t.Fatalf("RunCommand output mismatch: %s", outStr)
	}
}
