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
	} `yaml:"map_service"`

	// Valkey holds the ephemeral presence state and the cached friend sets that
	// Profiles maintains (friends:{user_id}). Map reads those sets for privacy
	// filtering but never writes them.
	Valkey struct {
		Address string `yaml:"address"`
	} `yaml:"valkey"`

	// Postgres owns the `map` schema (map_objects with a PostGIS geography column
	// and a GiST index). No cross-schema FKs.
	Postgres struct {
		DSN string `yaml:"dsn"`
	} `yaml:"postgres"`

	// Map holds this service's tuning knobs.
	Map struct {
		// RadiusMeters is the LoadMap search radius for ST_DWithin. Locked at 5km.
		RadiusMeters int `yaml:"radius_meters"`
		// PresenceTTLSeconds is the ephemeral presence TTL. Locked at 900 (15 min).
		PresenceTTLSeconds int `yaml:"presence_ttl_seconds"`
		// JanitorIntervalSeconds is how often the presence janitor reconciles
		// object:*:visitors sets against live presence:{user} keys.
		JanitorIntervalSeconds int `yaml:"janitor_interval_seconds"`
	} `yaml:"map"`
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
