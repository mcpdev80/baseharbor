package config

import "fmt"

const DefaultFile = "baseharbor.yaml"

type Config struct {
	Version    int
	Deployment string
	DataDir    string
	Runtime    string
}

func Default() Config {
	return Config{
		Version:    1,
		Deployment: "single-node",
		DataDir:    "./data",
		Runtime:    "auto",
	}
}

func (c Config) YAML() string {
	return fmt.Sprintf("version: %d\ndeployment: %s\ndata_dir: %s\nruntime: %s\n", c.Version, c.Deployment, c.DataDir, c.Runtime)
}
