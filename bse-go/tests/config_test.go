package main

import (
	"bse-go/pkg/config"
	"testing"
)

func TestConfigLoad(t *testing.T) {
	cfg, err := config.LoadConfig("../config/config.json")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Multicast.IP != "239.1.2.5" {
		t.Errorf("Expected IP 239.1.2.5, got %s", cfg.Multicast.IP)
	}

	if cfg.Multicast.Port != 26002 {
		t.Errorf("Expected port 26002, got %d", cfg.Multicast.Port)
	}
}
