package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitConfig_LoadsLocalValues(t *testing.T) {
	// The shipped values_local.yaml must decode into the Config struct with the
	// keys the App expects (ports + store addresses).
	path := filepath.Join("..", "..", "..", "configs", "values_local.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("config file not found: %v", err)
	}
	c, err := InitConfig(path)
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}
	if c.Service.GrpcPort == "" || c.Service.HttpPort == "" {
		t.Fatalf("service ports not loaded: %+v", c.Service)
	}
	if c.Postgres.DSN == "" {
		t.Fatal("postgres DSN not loaded")
	}
	if c.Valkey.Address == "" {
		t.Fatal("valkey address not loaded")
	}
}

func TestInitConfig_MissingFile(t *testing.T) {
	if _, err := InitConfig("/no/such/file.yaml"); err == nil {
		t.Fatal("expected error for missing config file")
	}
}
