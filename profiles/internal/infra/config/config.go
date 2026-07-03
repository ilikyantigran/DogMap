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
	} `yaml:"profiles_service"`

	// Long-term storage owned by Profiles (the `profiles` schema).
	Postgres struct {
		DSN string `yaml:"dsn"`
	} `yaml:"postgres"`

	// Valkey holds the `friends:{uid}` cache Profiles keeps fresh for Map, and is
	// read (read-only) for `session:{token}` to resolve the acting user. The
	// session key namespace is owned by Auth.
	Valkey struct {
		Address string `yaml:"address"`
	} `yaml:"valkey"`
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
