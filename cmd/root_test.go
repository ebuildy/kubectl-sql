package cmd

import "testing"

func TestValidateUIAddress(t *testing.T) {
	valid := []string{
		"127.0.0.1:8080",
		"0.0.0.0:9090",
		"localhost:1",
		"[::1]:65535",
		":8080",
	}
	for _, addr := range valid {
		if err := validateUIAddress(addr); err != nil {
			t.Errorf("validateUIAddress(%q) = %v, want nil", addr, err)
		}
	}

	invalid := []string{
		"127.0.0.1",       // no port
		"127.0.0.1:",      // empty port
		"127.0.0.1:abc",   // non-numeric port
		"127.0.0.1:0",     // out of range (low)
		"127.0.0.1:70000", // out of range (high)
		"",                // empty
	}
	for _, addr := range invalid {
		if err := validateUIAddress(addr); err == nil {
			t.Errorf("validateUIAddress(%q) = nil, want error", addr)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	loopback := []string{"localhost", "127.0.0.1", "::1"}
	for _, h := range loopback {
		if !isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = false, want true", h)
		}
	}
	nonLoopback := []string{"", "0.0.0.0", "192.168.1.10"}
	for _, h := range nonLoopback {
		if isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = true, want false", h)
		}
	}
}
