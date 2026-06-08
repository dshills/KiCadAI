package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const (
	EnvSocketPath = "KICAD_API_SOCKET"
	EnvToken      = "KICAD_API_TOKEN"
	EnvClientName = "KICAD_CLIENT_NAME"
	EnvTimeoutMS  = "KICAD_TIMEOUT_MS"

	DefaultTimeoutMS = 2000
)

type Explicit struct {
	SocketPath  string
	Token       string
	ClientName  string
	TimeoutMS   int
	Environment []string
}

type Config struct {
	SocketPath string `json:"socket_path"`
	Token      string `json:"token"`
	ClientName string `json:"client_name"`
	TimeoutMS  int    `json:"timeout_ms"`
}

func Resolve(explicit Explicit) (Config, error) {
	env := environMap(explicit.Environment)

	cfg := Config{
		SocketPath: firstNonEmpty(explicit.SocketPath, env[EnvSocketPath], defaultSocketPath()),
		Token:      firstNonEmpty(explicit.Token, env[EnvToken]),
		ClientName: firstNonEmpty(explicit.ClientName, env[EnvClientName], defaultClientName()),
		TimeoutMS:  DefaultTimeoutMS,
	}

	if explicit.TimeoutMS > 0 {
		cfg.TimeoutMS = explicit.TimeoutMS
	} else if raw := strings.TrimSpace(env[EnvTimeoutMS]); raw != "" {
		timeoutMS, err := strconv.Atoi(raw)
		if err != nil || timeoutMS <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive integer", EnvTimeoutMS)
		}

		cfg.TimeoutMS = timeoutMS
	}

	if strings.TrimSpace(cfg.SocketPath) == "" {
		return Config{}, fmt.Errorf("%s is required and no platform default is available", EnvSocketPath)
	}

	return cfg, nil
}

func (c Config) Redacted() Config {
	redacted := c

	if redacted.Token != "" {
		redacted.Token = "<redacted>"
	}

	return redacted
}

func defaultSocketPath() string {
	if runtime.GOOS == "windows" {
		return ""
	}

	return "ipc:///tmp/kicad/api.sock"
}

func defaultClientName() string {
	return fmt.Sprintf("kicadai-go-%d", os.Getpid())
}

func environMap(values []string) map[string]string {
	result := make(map[string]string, len(values))

	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if ok {
			result[key] = val
		}
	}

	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
