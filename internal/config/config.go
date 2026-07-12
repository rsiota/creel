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
	Settings    Settings           `yaml:"settings,omitempty"`
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

	appDir := filepath.Join(configDir, "gsql")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", fmt.Errorf("could not create config directory: %w", err)
	}

	return filepath.Join(appDir, "config.yaml"), nil
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
