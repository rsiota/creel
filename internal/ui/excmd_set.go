package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rsiota/creel/internal/config"
)

// settingSpec describes one runtime-tunable config field exposed via :set.
type settingSpec struct {
	key      string
	aliases  []string
	describe func(m Model) string
	apply    func(m *Model, value string) tea.Cmd
	complete func(m *Model) []string
}

var allSettingSpecs = []settingSpec{
	{
		key: "transparent_background",
		aliases: []string{
			"transparent", "transbg", "transparent-bg", "transparentbackground",
		},
		describe: func(m Model) string {
			return formatSettingBool("transparent_background", m.settings.TransparentBackground)
		},
		apply: func(m *Model, value string) tea.Cmd {
			on, ok := parseSettingBool(value)
			if !ok {
				m.schemaMsg = ":set transparent_background needs on or off"
				return nil
			}
			m.settings.TransparentBackground = on
			m.saveSettings()
			m.schemaMsg = formatSettingBool("transparent_background", on)
			return nil
		},
		complete: completeBoolValues,
	},
	{
		key:     "confirm_destructive",
		aliases: []string{"confirm", "confirm-destructive", "confirmdestructive"},
		describe: func(m Model) string {
			return "confirm_destructive=" + formatConfirmDestructive(m.settings.ConfirmDestructive)
		},
		apply: func(m *Model, value string) tea.Cmd {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "default":
				m.settings.ConfirmDestructive = nil
				m.saveSettings()
				m.schemaMsg = "confirm_destructive=default (on)"
				return nil
			}
			on, ok := parseSettingBool(value)
			if !ok {
				m.schemaMsg = ":set confirm_destructive needs on, off, or default"
				return nil
			}
			if on {
				m.settings.ConfirmDestructive = nil
				m.schemaMsg = "confirm_destructive=on"
			} else {
				v := false
				m.settings.ConfirmDestructive = &v
				m.schemaMsg = "confirm_destructive=off"
			}
			m.saveSettings()
			return nil
		},
		complete: func(_ *Model) []string {
			return []string{"on", "off", "default"}
		},
	},
	{
		key:     "page_size",
		aliases: []string{"pagesize", "page-size"},
		describe: func(m Model) string {
			return fmt.Sprintf("page_size=%d", m.settings.PageSize)
		},
		apply: func(m *Model, value string) tea.Cmd {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 1 {
				m.schemaMsg = ":set page_size needs a positive number"
				return nil
			}
			m.settings.PageSize = n
			m.pageSize = n
			m.saveSettings()
			m.schemaMsg = fmt.Sprintf("page_size=%d", n)
			if m.connection != nil && m.lastQuery != "" {
				m.page = 0
				return m.runPageQuery()
			}
			return nil
		},
	},
	{
		key:     "query_timeout",
		aliases: []string{"timeout", "query-timeout", "querytimeout"},
		describe: func(m Model) string {
			return "query_timeout=" + formatSettingDuration(m.settings.QueryTimeout)
		},
		apply: func(m *Model, value string) tea.Cmd {
			d, err := parseSettingDuration(value)
			if err != nil {
				m.schemaMsg = ":set query_timeout needs a duration (30s, 2m, off)"
				return nil
			}
			m.settings.QueryTimeout = d
			m.queryTimeout = d.Std()
			m.saveSettings()
			m.schemaMsg = "query_timeout=" + formatSettingDuration(d)
			return nil
		},
		complete: func(_ *Model) []string {
			return []string{"off", "30s", "1m", "2m", "5m"}
		},
	},
	{
		key:     "default_driver",
		aliases: []string{"driver", "default-driver", "defaultdriver"},
		describe: func(m Model) string {
			return "default_driver=" + m.settings.DefaultDriver
		},
		apply: func(m *Model, value string) tea.Cmd {
			driver := strings.ToLower(strings.TrimSpace(value))
			switch driver {
			case "sqlite", "mysql", "postgres":
			default:
				m.schemaMsg = ":set default_driver needs sqlite, mysql, or postgres"
				return nil
			}
			m.settings.DefaultDriver = driver
			m.saveSettings()
			m.schemaMsg = "default_driver=" + driver
			return nil
		},
		complete: func(_ *Model) []string {
			return []string{"sqlite", "mysql", "postgres"}
		},
	},
	{
		key: "theme",
		describe: func(m Model) string {
			name := m.settings.Theme
			if name == "" {
				name = defaultThemeName
			}
			return "theme=" + name
		},
		apply: func(m *Model, value string) tea.Cmd {
			return m.exTheme(value)
		},
		complete: func(_ *Model) []string {
			return themeNames()
		},
	},
	{
		key: "icons",
		describe: func(m Model) string {
			label := m.settings.Icons
			if label == "" {
				label = "unicode"
			}
			return "icons=" + label
		},
		apply: func(m *Model, value string) tea.Cmd {
			return m.exIcons(value)
		},
		complete: func(_ *Model) []string {
			return []string{"unicode", "nerdfont"}
		},
	},
	{
		key:     "inspector_open",
		aliases: []string{"inspector", "inspector-open", "inspectoropen", "show_inspector"},
		describe: func(m Model) string {
			return formatSettingBool("inspector_open", m.settings.InspectorOpen)
		},
		apply: func(m *Model, value string) tea.Cmd {
			on, ok := parseSettingBool(value)
			if !ok {
				m.schemaMsg = ":set inspector_open needs on or off"
				return nil
			}
			m.settings.InspectorOpen = on
			m.saveSettings()
			m.applyInspectorOpen(on, true)
			m.schemaMsg = formatSettingBool("inspector_open", on)
			return nil
		},
		complete: completeBoolValues,
	},
}

func lookupSetting(name string) *settingSpec {
	key := normalizeSettingKey(name)
	if key == "" {
		return nil
	}
	for i := range allSettingSpecs {
		spec := &allSettingSpecs[i]
		if spec.key == key {
			return spec
		}
		for _, alias := range spec.aliases {
			if normalizeSettingKey(alias) == key {
				return spec
			}
		}
	}
	return nil
}

func normalizeSettingKey(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.ReplaceAll(key, "-", "_")
	return key
}

func allSettingKeys() []string {
	keys := make([]string, len(allSettingSpecs))
	for i, s := range allSettingSpecs {
		keys[i] = s.key
	}
	return keys
}

// exSet views or changes a config setting (:set [option] [value]). Changes
// apply immediately and are persisted to ~/.config/creel/config.yaml.
func (m *Model) exSet(args []string) tea.Cmd {
	if len(args) == 0 {
		parts := make([]string, len(allSettingSpecs))
		for i, spec := range allSettingSpecs {
			parts[i] = spec.describe(*m)
		}
		m.schemaMsg = strings.Join(parts, "  ")
		return nil
	}
	spec := lookupSetting(args[0])
	if spec == nil {
		m.schemaMsg = fmt.Sprintf("unknown setting: %s (try :set)", args[0])
		return nil
	}
	if len(args) == 1 {
		m.schemaMsg = spec.describe(*m)
		return nil
	}
	return spec.apply(m, args[1])
}

func (m *Model) saveSettings() {
	if m.config == nil {
		return
	}
	m.config.Settings = m.settings
	_ = m.config.Save()
}

func completeSet(m *Model, args []string, partial string) []string {
	if len(args) == 0 {
		return rankStrings(partial, allSettingKeys())
	}
	spec := lookupSetting(args[0])
	if spec == nil || len(args) > 1 {
		return nil
	}
	if spec.complete == nil {
		return nil
	}
	return rankStrings(partial, spec.complete(m))
}

func completeBoolValues(_ *Model) []string {
	return []string{"on", "off", "true", "false"}
}

func formatSettingBool(name string, on bool) string {
	if on {
		return name + "=on"
	}
	return name + "=off"
}

func formatConfirmDestructive(v *bool) string {
	if v == nil {
		return "on (default)"
	}
	if *v {
		return "on"
	}
	return "off"
}

func parseSettingBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "yes", "1":
		return true, true
	case "off", "false", "no", "0":
		return false, true
	default:
		return false, false
	}
}

func parseSettingDuration(value string) (config.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "none", "disable", "disabled":
		return -1, nil
	}
	if d, err := time.ParseDuration(value); err == nil {
		return config.Duration(d), nil
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		return config.Duration(time.Duration(secs) * time.Second), nil
	}
	return 0, fmt.Errorf("invalid duration %q", value)
}

func formatSettingDuration(d config.Duration) string {
	if d.Std() < 0 {
		return "off"
	}
	return d.Std().String()
}
