package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config is loaded once at startup from the YAML file named by CONFIG_PATH.
// Keep it data-only: addresses, ports, and tuning knobs. No secrets in the file —
// anything sensitive comes from the environment. The two config files
// (configs/values_local.yaml and configs/values_docker.yaml) must share the same
// keys and differ only in values.
type Config struct {
	Service struct {
		Host     string `yaml:"host"` // host the HTTP gateway dials its own gRPC server on
		GrpcPort string `yaml:"grpc_port"`
		HttpPort string `yaml:"http_port"`
	} `yaml:"auth_service"`

	// Postgres holds Auth's own `auth` schema (credentials only, no cross-schema FKs).
	Postgres struct {
		DSN string `yaml:"dsn"`
	} `yaml:"postgres"`

	// Valkey stores opaque session tokens: session:{token} -> {user_id, exp}.
	Valkey struct {
		Address string `yaml:"address"`
	} `yaml:"valkey"`

	// Downstreams are gRPC addresses (host:port) of the services Auth calls.
	Downstreams struct {
		Profiles string `yaml:"profiles"` // CreateProfile handoff on register
	} `yaml:"downstreams"`

	// Auth-specific tuning knobs.
	Auth struct {
		SessionTTLSeconds int `yaml:"session_ttl_seconds"` // sliding session TTL (e.g. 86400 = 24h)
		// Argon2id parameters. Defaults applied in the hasher if zero.
		Argon2Memory      uint32 `yaml:"argon2_memory_kib"` // KiB
		Argon2Iterations  uint32 `yaml:"argon2_iterations"`
		Argon2Parallelism uint8  `yaml:"argon2_parallelism"`
	} `yaml:"auth"`
}

// InitConfig opens the YAML file at path and decodes it into a Config.
func InitConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := &Config{}
	if err = yaml.NewDecoder(f).Decode(c); err != nil {
		return nil, err
	}
	return c, nil
}
