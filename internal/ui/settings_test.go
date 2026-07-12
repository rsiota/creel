package ui

import (
	"testing"
	"time"

	"github.com/ruben/gsql/internal/config"
)

// NewModel applies effective settings: page size, query timeout, and the
// default driver for new connections.
func TestNewModelAppliesSettings(t *testing.T) {
	cfg := &config.Config{
		Settings: config.Settings{
			PageSize:      777,
			QueryTimeout:  config.Duration(9 * time.Second),
			DefaultDriver: "postgres",
		},
	}

	m := NewModel(cfg)

	if m.pageSize != 777 {
		t.Errorf("pageSize=%d, want 777", m.pageSize)
	}
	if m.queryTimeout != 9*time.Second {
		t.Errorf("queryTimeout=%v, want 9s", m.queryTimeout)
	}
	if m.settings.DefaultDriver != "postgres" {
		t.Errorf("settings.DefaultDriver=%q, want postgres", m.settings.DefaultDriver)
	}
}

// Missing settings fall back to the config defaults.
func TestNewModelDefaultsWhenSettingsAbsent(t *testing.T) {
	m := NewModel(&config.Config{})
	if m.pageSize != config.DefaultPageSize {
		t.Errorf("pageSize=%d, want %d", m.pageSize, config.DefaultPageSize)
	}
	if m.queryTimeout != config.DefaultQueryTimeout {
		t.Errorf("queryTimeout=%v, want %v", m.queryTimeout, config.DefaultQueryTimeout)
	}
	if m.settings.DefaultDriver != config.DefaultDriver {
		t.Errorf("DefaultDriver=%q, want %q", m.settings.DefaultDriver, config.DefaultDriver)
	}
}

// Opening the add-connection form seeds it with the configured default driver.
func TestAddConnectionUsesDefaultDriver(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{DefaultDriver: "mysql"}}
	m := NewModel(cfg)
	// The connections list is up; pressing 'n' opens the add-connection form.
	// Simulate that by constructing the form as the app does.
	m.connForm = NewConnectionForm()
	m.connForm.setDriverField(m.settings.DefaultDriver)

	if got := m.connForm.fields[fieldDriver].Value(); got != "mysql" {
		t.Errorf("add-form driver=%q, want mysql", got)
	}
}

// An unrecognized default driver falls back to sqlite rather than leaving the
// form in an invalid state.
func TestAddConnectionInvalidDriverFallsBack(t *testing.T) {
	cfg := &config.Config{Settings: config.Settings{DefaultDriver: "oracle"}}
	m := NewModel(cfg)
	m.connForm = NewConnectionForm()
	m.connForm.setDriverField(m.settings.DefaultDriver)

	if got := m.connForm.fields[fieldDriver].Value(); got != "sqlite" {
		t.Errorf("add-form driver=%q, want sqlite fallback", got)
	}
}
