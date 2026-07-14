package config

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Default settings. These are the zero-value fallbacks applied by
// Settings.Effective, so a config file with no "settings:" block (or one that
// leaves fields blank) always gets sane behaviour.
const (
	DefaultPageSize     = 200
	DefaultDriver       = "sqlite"
	DefaultQueryTimeout = 30 * time.Second
)

// Settings holds app-level preferences read from the "settings:" block of the
// config file. Zero values fall back to the defaults above (see Effective).
//
// Currently wired: page_size, query_timeout, default_driver, theme,
// transparent_background, confirm_destructive.
// Reserved for follow-ups (not yet applied): cursor_style — the struct is
// designed so adding fields is the only change needed here.
type Settings struct {
	PageSize      int      `yaml:"page_size,omitempty"`
	QueryTimeout  Duration `yaml:"query_timeout,omitempty"`
	DefaultDriver string   `yaml:"default_driver,omitempty"`

	// Theme selects the color palette (tokyo-night, gruvbox, nord, catppuccin,
	// light). Empty or unknown values fall back to the default theme at apply
	// time, so it is left unsanitized here to avoid sprouting `theme:` on
	// every config save.
	Theme string `yaml:"theme,omitempty"`

	// TransparentBackground leaves the app's background unpainted so the
	// terminal's own background (or window transparency / background image)
	// shows through. By default gsql fills the background with the active
	// theme's bg colour — necessary for light themes, whose foreground
	// palettes are unreadable on a dark terminal. Set this to make light
	// themes look wrong in exchange for keeping transparency; the value is
	// left unsanitized because false (the zero value) is the desired default.
	TransparentBackground bool `yaml:"transparent_background,omitempty"`

	// ConfirmDestructive gates the destructive-action confirmation dialogs
	// (drop table/database, truncate, delete rows, discard edits, drop column,
	// clear history/bookmarks). nil (the default) and true keep the prompts;
	// false runs each action immediately with no prompt. It is a pointer so the
	// safe default (confirm) differs from the zero value of bool.
	ConfirmDestructive *bool `yaml:"confirm_destructive,omitempty"`
}

// Effective returns a copy of s with zero-values replaced by the defaults, so
// callers never handle "unset" themselves.
func (s Settings) Effective() Settings {
	out := s
	if out.PageSize <= 0 {
		out.PageSize = DefaultPageSize
	}
	if out.QueryTimeout <= 0 {
		out.QueryTimeout = Duration(DefaultQueryTimeout)
	}
	if out.DefaultDriver == "" {
		out.DefaultDriver = DefaultDriver
	}
	return out
}

// Duration is a time.Duration that serializes to/from YAML in the friendly form
// produced by time.ParseDuration / time.Duration.String — e.g. "30s", "2m",
// "1h30m". A bare number is read as seconds, so `query_timeout: 30` works too.
type Duration time.Duration

// Std returns the value as a stdlib time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// MarshalYAML emits the duration in its friendly string form.
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// UnmarshalYAML accepts a friendly duration string ("30s", "2m", "1h") or a
// bare number of seconds (int or numeric string).
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var raw interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case string:
		if parsed, err := time.ParseDuration(v); err == nil {
			*d = Duration(parsed)
			return nil
		}
		if secs, err := strconv.Atoi(v); err == nil {
			*d = Duration(time.Duration(secs) * time.Second)
			return nil
		}
		return fmt.Errorf("invalid duration %q: use a value like 30s, 2m, or 1h", v)
	case int:
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	case int64:
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	default:
		return fmt.Errorf("invalid duration %v", raw)
	}
}
