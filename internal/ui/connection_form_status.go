package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// testState is the per-field outcome of a connection test. The zero value
// (testNeutral) means the field was not exercised — e.g. DB fields when the
// SSH tunnel failed first, so the DB handshake never ran.
type testState int

const (
	testNeutral testState = iota // untested / unknown
	testOK                       // field contributed to a successful connection
	testFail                     // field likely caused the failure
)

// statusOf returns the recorded test outcome for field fi (testNeutral if no
// test has run, or the field was not exercised).
func (f ConnectionForm) statusOf(fi int) testState {
	if f.testStates == nil {
		return testNeutral
	}
	return f.testStates[fi] // absent → testNeutral (zero value)
}

// fieldTestMarker returns the right-aligned label marker for a field's test
// state: a green ✓ for OK, a red ✗ for fail, nothing when untested. Markers
// stay on every attributed field; only fails get border/wash chrome.
func fieldTestMarker(s testState) string {
	switch s {
	case testOK:
		return lipgloss.NewStyle().Foreground(colorSuccess).Render("✓")
	case testFail:
		return lipgloss.NewStyle().Foreground(colorError).Render("✗")
	}
	return ""
}

// formFieldBorder picks the border style for a connection-form field: error
// red when the field failed the last test, otherwise the usual focus colouring
// (primary when active, grey otherwise). Passing fields stay neutral so light
// themes don't turn into a patchwork of green boxes — the ✓ marker is enough.
func formFieldBorder(focused bool, st testState) lipgloss.Style {
	if st == testFail {
		return lipgloss.NewStyle().Foreground(colorError)
	}
	return fieldBoxBorder(focused)
}

// applyFieldFailWash pads content to valueWidth and paints a soft error wash
// behind it so failed ctrl+t fields read clearly on light themes.
func applyFieldFailWash(content string, valueWidth int) string {
	if valueWidth < 1 {
		return content
	}
	w := lipgloss.Width(content)
	if w < valueWidth {
		content += strings.Repeat(" ", valueWidth-w)
	} else if w > valueWidth {
		content = ansi.Truncate(content, valueWidth, "…")
	}
	return lipgloss.NewStyle().Background(colorTestFailWash).Render(content)
}

// visibleFieldSet returns every field relevant to the current driver/SSH
// config (across both pages) as a set, for fast membership checks during
// classification.
func (f ConnectionForm) visibleFieldSet() map[int]bool {
	out := make(map[int]bool, len(f.fields))
	for _, fi := range f.relevantFields() {
		out[fi] = true
	}
	return out
}

// isSSHField reports whether fi is one of the SSH-tunnel parameter fields.
func isSSHField(fi int) bool {
	switch fi {
	case fieldSSHHost, fieldSSHPort, fieldSSHUser, fieldSSHKeyPath, fieldSSHPassword:
		return true
	}
	return false
}

// classifyTestError attributes a connection-test error to specific form fields
// so the UI can mark them with ✓/✗ (and fail-only border/wash). The connect
// path runs in stages — SSH tunnel (if any) first, then the DB network/auth
// handshake — so the stage that failed is the strongest signal and needs no
// string parsing: an SSH-stage failure means the DB fields were never tried
// (left neutral). Within the DB stage the error message is matched against
// stable driver/ssh strings to pinpoint the network (host/port), credentials
// (user/pass), or the database name. A nil error marks every visible field OK.
func (f ConnectionForm) classifyTestError(err error) map[int]testState {
	vis := f.visibleFieldSet()
	out := make(map[int]testState, len(vis))

	if err == nil {
		for fi := range vis {
			out[fi] = testOK
		}
		return out
	}

	// SQLite: the only connection field is the database path, so any failure
	// points at it.
	if !isNetworkDriver(f.driver()) {
		if vis[fieldDatabase] {
			out[fieldDatabase] = testFail
		}
		return out
	}

	msg := strings.ToLower(err.Error())

	// --- SSH stage -------------------------------------------------------
	// Every SSH-stage error flows through NewSSHTunnel / crypto/ssh and carries
	// an "ssh" marker. Since the tunnel is attempted before the DB handshake,
	// a tunnel failure means the DB fields are untested (left neutral).
	if f.sshEnabled() && strings.Contains(msg, "ssh") {
		switch {
		case strings.Contains(msg, "dial"):
			// Couldn't reach the bastion host.
			if vis[fieldSSHHost] {
				out[fieldSSHHost] = testFail
			}
			if vis[fieldSSHPort] {
				out[fieldSSHPort] = testFail
			}
		case strings.Contains(msg, "key") || strings.Contains(msg, "passphrase"):
			// Key file missing/unreadable or wrong passphrase.
			if vis[fieldSSHKeyPath] {
				out[fieldSSHKeyPath] = testFail
			}
		case strings.Contains(msg, "handshake") || strings.Contains(msg, "auth"):
			// Reached the bastion but credentials were rejected.
			if vis[fieldSSHUser] {
				out[fieldSSHUser] = testFail
			}
			if vis[fieldSSHKeyPath] {
				out[fieldSSHKeyPath] = testFail
			}
			if vis[fieldSSHPassword] {
				out[fieldSSHPassword] = testFail
			}
		default:
			// Unrecognized SSH error: flag the whole tunnel group.
			for fi := range vis {
				if isSSHField(fi) {
					out[fi] = testFail
				}
			}
		}
		return out
	}

	// --- DB stage --------------------------------------------------------
	// If a tunnel is configured but we reached the DB stage, the tunnel worked:
	// mark its fields OK before attributing the DB-side failure below.
	if f.sshEnabled() {
		for fi := range vis {
			if isSSHField(fi) {
				out[fi] = testOK
			}
		}
	}

	switch {
	case strings.Contains(msg, "access denied") ||
		strings.Contains(msg, "authentication failed"):
		// Reached the server; credentials rejected. Host/port were fine.
		if vis[fieldHost] {
			out[fieldHost] = testOK
		}
		if vis[fieldPort] {
			out[fieldPort] = testOK
		}
		if vis[fieldSSLMode] {
			out[fieldSSLMode] = testOK
		}
		if vis[fieldUser] {
			out[fieldUser] = testFail
		}
		if vis[fieldPass] {
			out[fieldPass] = testFail
		}

	case strings.Contains(msg, "unknown database") ||
		strings.Contains(msg, "does not exist"):
		// Authenticated fine; the named database is missing.
		if vis[fieldHost] {
			out[fieldHost] = testOK
		}
		if vis[fieldPort] {
			out[fieldPort] = testOK
		}
		if vis[fieldUser] {
			out[fieldUser] = testOK
		}
		if vis[fieldPass] {
			out[fieldPass] = testOK
		}
		if vis[fieldSSLMode] {
			out[fieldSSLMode] = testOK
		}
		if vis[fieldDatabase] {
			out[fieldDatabase] = testFail
		}

	case strings.Contains(msg, "ssl") || strings.Contains(msg, "tls") ||
		strings.Contains(msg, "x509") || strings.Contains(msg, "certificate"):
		if vis[fieldSSLMode] {
			out[fieldSSLMode] = testFail
		}

	case vis[fieldSocket] && f.fields[fieldSocket].Value() != "" &&
		(strings.Contains(msg, "socket") || strings.Contains(msg, "unix") ||
			strings.Contains(msg, "no such file")):
		if vis[fieldSocket] {
			out[fieldSocket] = testFail
		}

	default:
		// Network/availability failure: connection refused, no such host,
		// timeout, etc. Credentials were never checked (left neutral).
		if vis[fieldHost] {
			out[fieldHost] = testFail
		}
		if vis[fieldPort] {
			out[fieldPort] = testFail
		}
	}
	return out
}
