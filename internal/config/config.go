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
	RuntimeOS   string
}

type Config struct {
	SocketPath string `json:"socket_path"`
	Token      string `json:"token"`
	ClientName string `json:"client_name"`
	TimeoutMS  int    `json:"timeout_ms"`
}

func Resolve(explicit Explicit) (Config, error) {
	env := environMap(explicit.Environment)
	runtimeOS := firstNonEmpty(strings.TrimSpace(explicit.RuntimeOS), runtime.GOOS)
	socketPath := strings.TrimSpace(explicit.SocketPath)
	envSocketPath := strings.TrimSpace(env[EnvSocketPath])
	token := strings.TrimSpace(explicit.Token)
	envToken := strings.TrimSpace(env[EnvToken])
	clientName := strings.TrimSpace(explicit.ClientName)
	envClientName := strings.TrimSpace(env[EnvClientName])

	if explicit.SocketPath != "" && socketPath == "" {
		return Config{}, fmt.Errorf("socket path must not be blank")
	}

	cfg := Config{
		SocketPath: firstNonEmpty(socketPath, envSocketPath, defaultSocketPath(runtimeOS)),
		Token:      firstNonEmpty(token, envToken),
		ClientName: firstNonEmpty(clientName, envClientName, defaultClientName()),
		TimeoutMS:  DefaultTimeoutMS,
	}

	if explicit.TimeoutMS < 0 {
		return Config{}, fmt.Errorf("timeout must not be negative")
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

	cfg.SocketPath = normalizeSocketPath(cfg.SocketPath, runtimeOS)

	return cfg, nil
}

func (c Config) Redacted() Config {
	redacted := c

	if redacted.Token != "" {
		redacted.Token = "<redacted>"
	}

	return redacted
}

func NormalizeSocketPath(socketPath string, runtimeOS ...string) string {
	targetOS := runtime.GOOS
	if len(runtimeOS) > 0 && strings.TrimSpace(runtimeOS[0]) != "" {
		targetOS = strings.TrimSpace(runtimeOS[0])
	}

	return normalizeSocketPath(socketPath, targetOS)
}

func defaultSocketPath(runtimeOS string) string {
	if runtimeOS == "windows" {
		return ""
	}

	return "ipc:///tmp/kicad/api.sock"
}

func normalizeSocketPath(socketPath string, runtimeOS string) string {
	trimmed := strings.TrimSpace(socketPath)

	if trimmed == "" || strings.Contains(trimmed, "://") || runtimeOS == "windows" {
		return trimmed
	}

	return "ipc://" + trimmed
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
