package config

import "os"

type Config struct {
	DataDir    string
	ListenAddr string
}

func Load() (*Config, error) {
	return &Config{
		DataDir:    getenv("CASS_DATA_DIR", "/var/cass"),
		ListenAddr: getenv("CASS_LISTEN", "0.0.0.0:8443"),
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
