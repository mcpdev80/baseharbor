package runtime

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string
	PostgresPort     int
	OpenBaoPort      int
}

func LoadConfig(envPath string) (Config, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	values := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return Config{}, fmt.Errorf("invalid runtime environment line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := s.Err(); err != nil {
		return Config{}, err
	}

	port := func(key string) (int, error) {
		v := values[key]
		if v == "" {
			return 0, fmt.Errorf("%s is required", key)
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return 0, fmt.Errorf("%s must be a valid TCP port", key)
		}
		return n, nil
	}

	postgresPort, err := port("BASEHARBOR_POSTGRES_PORT")
	if err != nil {
		return Config{}, err
	}
	openBaoPort, err := port("BASEHARBOR_OPENBAO_PORT")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		PostgresDB:       values["BASEHARBOR_POSTGRES_DB"],
		PostgresUser:     values["BASEHARBOR_POSTGRES_USER"],
		PostgresPassword: values["BASEHARBOR_POSTGRES_PASSWORD"],
		PostgresPort:     postgresPort,
		OpenBaoPort:      openBaoPort,
	}
	if cfg.PostgresDB == "" || cfg.PostgresUser == "" || cfg.PostgresPassword == "" {
		return Config{}, fmt.Errorf("runtime PostgreSQL configuration is incomplete")
	}
	return cfg, nil
}
