package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	Connections []ConnectionConfig `yaml:"connections"`
	AI          AIConfig           `yaml:"ai,omitempty"`
	Settings    Settings           `yaml:"settings,omitempty"`
}

// AIConfig holds the AI provider configuration for the :ai / assistant
// panel. Providers is a named list (each bundles an API key, base URL, and
// default model); Default names the active one. With no providers configured
// the app falls back to the CREEL_AI_* environment variables, so this block is
// entirely optional.
type AIConfig struct {
	Default   string       `yaml:"default,omitempty"`
	Providers []AIProvider `yaml:"providers,omitempty"`
}

// AIProvider is one OpenAI-compatible endpoint. APIKey is either plaintext or
// a "secret://<key>" keychain reference (resolved at request time via
// internal/secrets), so the key can live in the OS keychain instead of the
// config file. BaseURL/Model fall back to the ai package defaults when empty.
type AIProvider struct {
	Name    string `yaml:"name"`
	APIKey  string `yaml:"api_key,omitempty"`
	BaseURL string `yaml:"base_url,omitempty"`
	Model   string `yaml:"model,omitempty"`
}

// ConnectionConfig mirrors db.ConnectionConfig but lives in config package
// to avoid circular imports. It is kept in sync via the ToDBConfig method.
type ConnectionConfig struct {
	Name     string `yaml:"name"`
	Driver   string `yaml:"driver"`
	Database string `yaml:"database"`
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// ReadOnly disables writes on this connection (see db.ConnectionConfig).
	ReadOnly bool `yaml:"readonly,omitempty"`

	// Group is an optional folder label used to group connections in the
	// connection list (e.g. "Work", "Personal"). Empty means ungrouped.
	Group string `yaml:"group,omitempty"`

	// SSH tunnel (optional)
	SSHHost       string `yaml:"ssh_host,omitempty"`
	SSHPort       int    `yaml:"ssh_port,omitempty"`
	SSHUser       string `yaml:"ssh_user,omitempty"`
	SSHPassword   string `yaml:"ssh_password,omitempty"`
	SSHKeyPath    string `yaml:"ssh_key_path,omitempty"`
	SSHPassphrase string `yaml:"ssh_passphrase,omitempty"`
}

// ConfigPath returns the path to the config file, creating the directory if needed.
func ConfigPath() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}

	appDir := filepath.Join(configDir, "creel")

	// One-time migration from the predecessor's config dir (gsql). If the new
	// dir doesn't exist yet but the legacy one does, move it over so existing
	// users keep their connections, history, bookmarks, and sessions. Rename is
	// atomic and instant when both paths share a filesystem (they do: same
	// parent). Skipped once the new dir exists, so it's safe on every run.
	migrateLegacyConfigDir(filepath.Join(configDir, "gsql"), appDir)

	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", fmt.Errorf("could not create config directory: %w", err)
	}

	return filepath.Join(appDir, "config.yaml"), nil
}

// migrateLegacyConfigDir moves legacyDir onto newDir when newDir is absent and
// legacyDir exists. Any error is reported on stderr but never fatal — the
// caller then creates a fresh newDir, and the legacy data is left untouched.
func migrateLegacyConfigDir(legacyDir, newDir string) {
	if _, err := os.Stat(newDir); err == nil {
		return // new dir already present (or created); nothing to migrate
	}
	if _, err := os.Stat(legacyDir); err != nil {
		return // no legacy dir to migrate from
	}
	if err := os.Rename(legacyDir, newDir); err != nil {
		// Cross-device rename (unlikely, same parent) or permission issue: leave
		// the legacy dir in place rather than risk losing data.
		fmt.Fprintf(os.Stderr, "creel: could not migrate %s -> %s (%v); leaving it in place\n", legacyDir, newDir, err)
		return
	}
	fmt.Fprintf(os.Stderr, "creel: migrated config from %s to %s\n", legacyDir, newDir)
}

// Load reads the config file from disk. If the file does not exist, it returns
// an empty Config.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("could not read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the config file to disk.
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}

	return nil
}

// AddConnection adds a new connection to the config.
func (c *Config) AddConnection(conn ConnectionConfig) {
	c.Connections = append(c.Connections, conn)
}

// RemoveConnection removes a connection by name.
func (c *Config) RemoveConnection(name string) {
	for i, conn := range c.Connections {
		if conn.Name == name {
			c.Connections = append(c.Connections[:i], c.Connections[i+1:]...)
			return
		}
	}
}

// GetConnection returns a connection by name, or nil if not found.
func (c *Config) GetConnection(name string) *ConnectionConfig {
	for i := range c.Connections {
		if c.Connections[i].Name == name {
			return &c.Connections[i]
		}
	}
	return nil
}

// AddAIProvider appends an AI provider to the config.
func (c *Config) AddAIProvider(p AIProvider) {
	c.AI.Providers = append(c.AI.Providers, p)
}

// RemoveAIProvider removes an AI provider by name. Removing the provider that
// is the configured default clears the default (the caller is expected to set
// a new one if any providers remain).
func (c *Config) RemoveAIProvider(name string) {
	for i, p := range c.AI.Providers {
		if p.Name == name {
			c.AI.Providers = append(c.AI.Providers[:i], c.AI.Providers[i+1:]...)
			if c.AI.Default == name {
				c.AI.Default = ""
			}
			return
		}
	}
}

// GetAIProvider returns an AI provider by name, or nil if not found.
func (c *Config) GetAIProvider(name string) *AIProvider {
	for i := range c.AI.Providers {
		if c.AI.Providers[i].Name == name {
			return &c.AI.Providers[i]
		}
	}
	return nil
}
