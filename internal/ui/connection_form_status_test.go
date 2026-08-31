package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// formForDriver returns a fresh form with the given driver (and SSH toggle).
func formForDriver(driver string, ssh bool) ConnectionForm {
	f := NewConnectionForm()
	f.fields[fieldDriver].SetValue(driver)
	if ssh {
		f.fields[fieldSSHTunnel].SetValue("yes")
	}
	return f
}

// A successful test marks every visible field OK.
func TestClassifySuccessAllGreen(t *testing.T) {
	f := formForDriver("mysql", true)
	got := f.classifyTestError(nil)
	for _, fi := range f.visibleFields() {
		if got[fi] != testOK {
			t.Errorf("field %d (%s): got %d, want testOK", fi, formLabels[fi], got[fi])
		}
	}
}

// SQLite has only one connection field: any failure points at the database.
func TestClassifySQLiteFailureFlagsDatabase(t *testing.T) {
	f := formForDriver("sqlite", false)
	got := f.classifyTestError(errors.New("unable to open database file: path/to/x.db"))
	if got[fieldDatabase] != testFail {
		t.Errorf("database: got %d, want testFail", got[fieldDatabase])
	}
	// Non-connection fields stay neutral (the map only records exercised fields).
	for fi := range got {
		if fi == fieldDatabase {
			continue
		}
		t.Errorf("unexpected attributed field %d (%s) for sqlite failure", fi, formLabels[fi])
	}
}

// MySQL "Access denied": host/port OK (server was reached), user/pass fail.
func TestClassifyMySQLAuthFailure(t *testing.T) {
	f := formForDriver("mysql", false)
	got := f.classifyTestError(errors.New("Error 1045 (28000): Access denied for user 'root'@'localhost'"))
	if got[fieldHost] != testOK {
		t.Errorf("host: got %d, want testOK", got[fieldHost])
	}
	if got[fieldPort] != testOK {
		t.Errorf("port: got %d, want testOK", got[fieldPort])
	}
	if got[fieldUser] != testFail {
		t.Errorf("user: got %d, want testFail", got[fieldUser])
	}
	if got[fieldPass] != testFail {
		t.Errorf("pass: got %d, want testFail", got[fieldPass])
	}
	if got[fieldDatabase] != testNeutral {
		t.Errorf("database should be neutral (not exercised): got %d", got[fieldDatabase])
	}
}

// Postgres "authentication failed": same attribution as MySQL access denied.
func TestClassifyPostgresAuthFailure(t *testing.T) {
	f := formForDriver("postgres", false)
	got := f.classifyTestError(errors.New(`pq: password authentication failed for user "postgres"`))
	if got[fieldUser] != testFail || got[fieldPass] != testFail {
		t.Errorf("want user/pass fail; got user=%d pass=%d", got[fieldUser], got[fieldPass])
	}
	if got[fieldHost] != testOK {
		t.Errorf("host should be OK (reached): got %d", got[fieldHost])
	}
}

// "Unknown database": authed fine, the named database is missing.
func TestClassifyUnknownDatabase(t *testing.T) {
	f := formForDriver("mysql", false)
	got := f.classifyTestError(errors.New("Unknown database 'myapp'"))
	if got[fieldDatabase] != testFail {
		t.Errorf("database: got %d, want testFail", got[fieldDatabase])
	}
	for _, fi := range []int{fieldHost, fieldPort, fieldUser, fieldPass} {
		if got[fi] != testOK {
			t.Errorf("%s: got %d, want testOK (auth/network were fine)", formLabels[fi], got[fi])
		}
	}
}

// A network failure (refused/no-such-host/timeout) flags host/port only;
// credentials were never checked so they stay neutral.
func TestClassifyNetworkFailureFlagsHostPort(t *testing.T) {
	f := formForDriver("mysql", false)
	got := f.classifyTestError(errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))
	if got[fieldHost] != testFail {
		t.Errorf("host: got %d, want testFail", got[fieldHost])
	}
	if got[fieldPort] != testFail {
		t.Errorf("port: got %d, want testFail", got[fieldPort])
	}
	if got[fieldUser] != testNeutral {
		t.Errorf("user should be neutral (never checked): got %d", got[fieldUser])
	}
}

// An SSH dial failure flags the bastion host/port, and leaves the DB fields
// neutral because the DB handshake never ran.
func TestClassifySSHDialFailure(t *testing.T) {
	f := formForDriver("mysql", true)
	got := f.classifyTestError(errors.New("ssh tunnel: ssh dial 1.2.3.4:22: dial tcp: connect: connection refused"))
	if got[fieldSSHHost] != testFail {
		t.Errorf("ssh host: got %d, want testFail", got[fieldSSHHost])
	}
	if got[fieldSSHPort] != testFail {
		t.Errorf("ssh port: got %d, want testFail", got[fieldSSHPort])
	}
	for _, fi := range []int{fieldHost, fieldPort, fieldUser, fieldPass} {
		if got[fi] != testNeutral {
			t.Errorf("%s should be neutral (DB stage never ran): got %d", formLabels[fi], got[fi])
		}
	}
}

// An SSH auth failure (handshake rejected) flags the SSH credentials.
func TestClassifySSHAuthFailure(t *testing.T) {
	f := formForDriver("mysql", true)
	got := f.classifyTestError(errors.New("ssh: handshake failed: ssh: unable to authenticate, no supported methods remain"))
	for _, fi := range []int{fieldSSHUser, fieldSSHKeyPath, fieldSSHPassword} {
		if got[fi] != testFail {
			t.Errorf("%s: got %d, want testFail", formLabels[fi], got[fi])
		}
	}
}

// An SSH key-file error flags only the key path.
func TestClassifySSHKeyFailure(t *testing.T) {
	f := formForDriver("mysql", true)
	got := f.classifyTestError(errors.New("ssh tunnel: ssh key /home/u/.ssh/id_rsa: open: no such file"))
	if got[fieldSSHKeyPath] != testFail {
		t.Errorf("ssh key: got %d, want testFail", got[fieldSSHKeyPath])
	}
	if got[fieldSSHHost] != testNeutral {
		t.Errorf("ssh host should be neutral (not a dial problem here): got %d", got[fieldSSHHost])
	}
}

// When the tunnel succeeds but the DB stage fails, the SSH fields are marked OK
// (they worked) rather than left neutral.
func TestClassifySSHSucceededThenDBFails(t *testing.T) {
	f := formForDriver("mysql", true)
	got := f.classifyTestError(errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))
	if got[fieldSSHHost] != testOK {
		t.Errorf("ssh host should be OK (tunnel worked): got %d", got[fieldSSHHost])
	}
	if got[fieldHost] != testFail {
		t.Errorf("db host should be fail: got %d", got[fieldHost])
	}
}

// The rendered form shows ✓ on passing fields and ✗ on failing ones after a
// test, and the markers disappear again once the user edits (clearTransient).
func TestFormRendersTestMarkersAndClearsOnEdit(t *testing.T) {
	f := formForDriver("mysql", false)
	f.SetSize(67, f.contentHeight())

	f.SetTestResult("✗ failed", errors.New("Error 1045: Access denied for user 'root'"))
	view := f.View()
	if !strings.Contains(view, "✗") {
		t.Errorf("expected ✗ marker on failing fields after a failed test\n%s", view)
	}
	if !strings.Contains(view, "✓") {
		t.Errorf("expected ✓ marker on the OK host/port fields\n%s", view)
	}

	// Start editing → transient result (markers included) must clear.
	f.editing = true
	f.clearTransient()
	view = f.View()
	if strings.Contains(view, "✓") || strings.Contains(view, "✗") {
		t.Errorf("markers should be cleared after editing\n%s", view)
	}
}

// A successful test shows ✓ on every visible field.
func TestFormRendersSuccessMarkers(t *testing.T) {
	f := formForDriver("sqlite", false)
	f.SetSize(67, f.contentHeight())
	f.SetTestResult("✓ Connected (sqlite)", nil)
	view := f.View()
	if c := strings.Count(view, "✓"); c < 2 {
		t.Errorf("expected ✓ on visible fields, got %d\n%s", c, view)
	}
	if strings.Contains(view, "✗") {
		t.Errorf("no ✗ expected on success\n%s", view)
	}
}

// Passing fields keep the normal focus/idle border; only fails use error red.
func TestFormFieldBorderFailOnly(t *testing.T) {
	okBorder := formFieldBorder(false, testOK).GetForeground()
	idleBorder := fieldBoxBorder(false).GetForeground()
	if okBorder != idleBorder {
		t.Errorf("testOK border = %v, want idle %v (no green chrome)", okBorder, idleBorder)
	}
	failBorder := formFieldBorder(false, testFail).GetForeground()
	if failBorder != colorError {
		t.Errorf("testFail border = %v, want colorError %v", failBorder, colorError)
	}
	focusedOK := formFieldBorder(true, testOK).GetForeground()
	focusedIdle := fieldBoxBorder(true).GetForeground()
	if focusedOK != focusedIdle {
		t.Errorf("focused testOK border = %v, want focus %v", focusedOK, focusedIdle)
	}
}

func TestApplyFieldFailWashPads(t *testing.T) {
	got := applyFieldFailWash("ab", 5)
	if lipgloss.Width(got) != 5 {
		t.Fatalf("width = %d, want 5", lipgloss.Width(got))
	}
	if plain := ansi.Strip(got); plain != "ab   " {
		t.Fatalf("plain = %q, want %q", plain, "ab   ")
	}
}

// Failed fields paint a soft error wash behind the value line; OK fields don't.
func TestFormFailFieldGetsWash(t *testing.T) {
	f := formForDriver("mysql", false)
	f.SetSize(67, f.contentHeight())
	f.fields[fieldUser].SetValue("root")
	f.SetTestResult("✗ failed", errors.New("Error 1045: Access denied for user 'root'"))
	washed := f.fieldValueContent(fieldUser, 40)
	f.testStates[fieldUser] = testOK
	neutral := f.fieldValueContent(fieldUser, 40)
	if washed == neutral {
		t.Fatal("fail wash should change the value-line rendering vs OK state")
	}
	if ansi.Strip(washed) != ansi.Strip(neutral) {
		t.Fatalf("wash should not change visible text: washed=%q neutral=%q",
			ansi.Strip(washed), ansi.Strip(neutral))
	}
}
