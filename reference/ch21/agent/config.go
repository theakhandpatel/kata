package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the resolved settings for one run.
//
// The fields are resolved from several places in a fixed order, and the resolved value is
// what the rest of the program sees. Nothing downstream reads the environment directly.
type Config struct {
	BaseURL string      `json:"base_url"`
	Model   string      `json:"model"`
	APIKey  string      `json:"api_key,omitempty"`
	KeyFile string      `json:"api_key_file,omitempty"`
	MaxSteps    int     `json:"max_steps,omitempty"`
	MaxContext  int     `json:"max_context,omitempty"`
	MaxTools    int     `json:"max_tools,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`

	// Sources records where each field came from, for `-config` to print.
	Sources map[string]string `json:"-"`
}

// FileConfig is what a config file may contain. It is a separate type from Config so that a
// missing field in a file means "not set here" rather than "set to the zero value".
type FileConfig struct {
	BaseURL     *string  `json:"base_url"`
	Model       *string  `json:"model"`
	APIKey      *string  `json:"api_key"`
	KeyFile     *string  `json:"api_key_file"`
	MaxSteps    *int     `json:"max_steps"`
	MaxContext  *int     `json:"max_context"`
	MaxTools    *int     `json:"max_tools"`
	Temperature *float64 `json:"temperature"`
}

// ConfigScope is one place settings may come from.
type ConfigScope struct {
	Name string
	Path string
}

// DefaultConfigScopes returns the files a run consults, lowest priority first.
//
// Project beats user, the same way project skills beat user skills: a repository's settings
// are about that repository.
func DefaultConfigScopes(projectRoot string) []ConfigScope {
	var scopes []ConfigScope

	if config := os.Getenv("XDG_CONFIG_HOME"); config != "" {
		scopes = append(scopes, ConfigScope{Name: "user", Path: filepath.Join(config, "kata", "agent.json")})
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		scopes = append(scopes, ConfigScope{Name: "user", Path: filepath.Join(home, ".config", "kata", "agent.json")})
	}
	scopes = append(scopes, ConfigScope{Name: "project", Path: filepath.Join(projectRoot, "agent.json")})
	return scopes
}

// LoadConfig resolves settings from the scopes, then the environment, then the flags.
//
// Later sources win. That order is deliberate: a file is shared and long-lived, an
// environment variable belongs to a shell, and a flag belongs to one command.
func LoadConfig(scopes []ConfigScope, env map[string]string, flags FileConfig) (Config, error) {
	cfg := Config{Sources: map[string]string{}}

	for _, scope := range scopes {
		file, err := readConfigFile(scope.Path)
		if err != nil {
			return cfg, err
		}
		if file == nil {
			continue
		}
		applyFile(&cfg, *file, scope.Name)
	}

	applyEnv(&cfg, env)
	applyFile(&cfg, flags, "flag")

	if err := resolveKey(&cfg); err != nil {
		return cfg, err
	}
	applyDefaults(&cfg)
	return cfg, nil
}

// readConfigFile returns nil when the file does not exist, which is the normal case.
func readConfigFile(path string) (*FileConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var file FileConfig
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &file, nil
}

func applyFile(cfg *Config, file FileConfig, source string) {
	if file.BaseURL != nil {
		cfg.BaseURL, cfg.Sources["base_url"] = *file.BaseURL, source
	}
	if file.Model != nil {
		cfg.Model, cfg.Sources["model"] = *file.Model, source
	}
	if file.APIKey != nil {
		cfg.APIKey, cfg.Sources["api_key"] = *file.APIKey, source
	}
	if file.KeyFile != nil {
		cfg.KeyFile, cfg.Sources["api_key_file"] = *file.KeyFile, source
	}
	if file.MaxSteps != nil {
		cfg.MaxSteps, cfg.Sources["max_steps"] = *file.MaxSteps, source
	}
	if file.MaxContext != nil {
		cfg.MaxContext, cfg.Sources["max_context"] = *file.MaxContext, source
	}
	if file.MaxTools != nil {
		cfg.MaxTools, cfg.Sources["max_tools"] = *file.MaxTools, source
	}
	if file.Temperature != nil {
		cfg.Temperature, cfg.Sources["temperature"] = *file.Temperature, source
	}
}

// applyEnv reads the variables the agent has always supported, so an existing shell keeps
// working.
func applyEnv(cfg *Config, env map[string]string) {
	set := func(value string, target *string, field string) {
		if value == "" {
			return
		}
		*target = value
		cfg.Sources[field] = "environment"
	}
	set(env["OPENAI_BASE_URL"], &cfg.BaseURL, "base_url")
	set(env["OPENAI_API_KEY"], &cfg.APIKey, "api_key")
	set(env["HARNESS_MODEL"], &cfg.Model, "model")
	set(env["KATA_API_KEY_FILE"], &cfg.KeyFile, "api_key_file")
}

// resolveKey prefers an inline key, then a file.
//
// A file is the better place for a secret: it can have 0600 permissions and it does not
// appear in a process listing or a shell history.
func resolveKey(cfg *Config) error {
	if cfg.APIKey != "" {
		return nil
	}
	if cfg.KeyFile == "" {
		return nil
	}

	home, _ := os.UserHomeDir()
	path := cfg.KeyFile
	if strings.HasPrefix(path, "~/") && home != "" {
		path = filepath.Join(home, path[2:])
	}

	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("api_key_file: %s does not exist", path)
		}
		return fmt.Errorf("api_key_file: %w", err)
	}
	key := strings.TrimSpace(string(body))
	if key == "" {
		return fmt.Errorf("api_key_file: %s is empty", path)
	}
	cfg.APIKey = key
	cfg.Sources["api_key"] = "file " + path
	return nil
}

func applyDefaults(cfg *Config) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434/v1"
		cfg.Sources["base_url"] = "default"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
		cfg.Sources["model"] = "default"
	}
	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = DefaultMaxSteps
	}
	if cfg.MaxContext == 0 {
		cfg.MaxContext = DefaultMaxContext
	}
	if cfg.MaxTools == 0 {
		cfg.MaxTools = DefaultMaxTools
	}
}

// ValidateBaseURL catches the two mistakes that produce unreadable transport errors.
//
// A URL with no scheme makes the client think the host is a path. A URL with no /v1 makes
// every request 404, which looks like a wrong route rather than a wrong setting.
func ValidateBaseURL(raw string) error {
	if raw == "" {
		return errors.New("base_url is empty")
	}
	if !strings.Contains(raw, "://") {
		return fmt.Errorf("base_url %q has no scheme, try https://%s", raw, raw)
	}
	if strings.HasSuffix(raw, "/chat/completions") {
		return fmt.Errorf("base_url %q includes the path; it should stop at /v1", raw)
	}
	return nil
}

// Describe renders the resolved settings with the key redacted.
//
// The report names where each value came from, because "why is it using that model" is the
// question a config file exists to answer.
func (c Config) Describe() []string {
	redact := func(s string) string {
		if s == "" {
			return "(none)"
		}
		if len(s) <= 8 {
			return "(set)"
		}
		return s[:4] + "…" + s[len(s)-4:]
	}
	source := func(field string) string {
		if s, ok := c.Sources[field]; ok {
			return s
		}
		return "unset"
	}

	return []string{
		fmt.Sprintf("base_url      %-42s (%s)", c.BaseURL, source("base_url")),
		fmt.Sprintf("model         %-42s (%s)", c.Model, source("model")),
		fmt.Sprintf("api_key       %-42s (%s)", redact(c.APIKey), source("api_key")),
		fmt.Sprintf("max_steps     %-42d", c.MaxSteps),
		fmt.Sprintf("max_context   %-42d", c.MaxContext),
		fmt.Sprintf("max_tools     %-42d", c.MaxTools),
		fmt.Sprintf("temperature   %-42g", c.Temperature),
	}
}
