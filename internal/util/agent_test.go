package util

import (
	"testing"
)

func TestAgentConfigParse(t *testing.T) {
	cfg, err := ParseAgentConfig("123", "./testdata/example1.ts")
	if err != nil {
		t.Fatalf("error parsing agent config: %s", err)
	}
	if cfg == nil {
		t.Fatalf("expected config to be non-nil")
	}
	if cfg.ID != "3c4c0b692533d7807cf0f649ef425dfa29b58bcc99be03e208e52749107fca2e" {
		t.Fatalf("expected id to be 3c4c0b692533d7807cf0f649ef425dfa29b58bcc99be03e208e52749107fca2e, got %s", cfg.ID)
	}
	if cfg.Name != "MyFirstAgent" {
		t.Fatalf("expected name to be MyFirstAgent, got %s", cfg.Name)
	}
	if cfg.Description != "A simple agent that can generate text" {
		t.Fatalf("expected description to be 'A simple agent that can generate text', got '%s'", cfg.Description)
	}
}

func TestAgentConfigParse2(t *testing.T) {
	cfg, err := ParseAgentConfig("123", "./testdata/example2.ts")
	if err != nil {
		t.Fatalf("error parsing agent config: %s", err)
	}
	if cfg == nil {
		t.Fatalf("expected config to be non-nil")
	}
	if cfg.ID != "3c4c0b692533d7807cf0f649ef425dfa29b58bcc99be03e208e52749107fca2e" {
		t.Fatalf("expected id to be 3c4c0b692533d7807cf0f649ef425dfa29b58bcc99be03e208e52749107fca2e, got %s", cfg.ID)
	}
	if cfg.Name != "MyFirstAgent" {
		t.Fatalf("expected name to be MyFirstAgent, got %s", cfg.Name)
	}
	if cfg.Description != "A simple agent that can generate text" {
		t.Fatalf("expected description to be 'A simple agent that can generate text', got '%s'", cfg.Description)
	}
}

func TestAgentConfigParse3(t *testing.T) {
	cfg, err := ParseAgentConfig("123", "./testdata/example3.ts")
	if err != nil {
		t.Fatalf("error parsing agent config: %s", err)
	}
	if cfg == nil {
		t.Fatalf("expected config to be non-nil")
	}
	if cfg.ID != "3c4c0b692533d7807cf0f649ef425dfa29b58bcc99be03e208e52749107fca2e" {
		t.Fatalf("expected id to be 3c4c0b692533d7807cf0f649ef425dfa29b58bcc99be03e208e52749107fca2e, got %s", cfg.ID)
	}
	if cfg.Name != "MyFirstAgent" {
		t.Fatalf("expected name to be MyFirstAgent, got %s", cfg.Name)
	}
	if cfg.Description != "A simple agent that can generate text" {
		t.Fatalf("expected description to be 'A simple agent that can generate text', got '%s'", cfg.Description)
	}
}
