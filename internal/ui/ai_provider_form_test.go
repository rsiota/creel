package ui

import (
	"testing"

	"github.com/rsiota/creel/internal/config"
)

func TestProviderFormValidation(t *testing.T) {
	f := NewProviderForm()
	f.Show()

	// Fresh form: name and key both empty.
	_, errMsg := f.EnterPressed()
	if errMsg == "" {
		t.Error("expected error for empty name")
	}

	// Name set, key empty.
	f.fields[pfName].SetValue("openai")
	_, errMsg = f.EnterPressed()
	if errMsg == "" {
		t.Error("expected error for empty api key")
	}

	// Fully valid.
	f.fields[pfKey].SetValue("sk-123")
	p, errMsg := f.EnterPressed()
	if errMsg != "" {
		t.Fatalf("expected no error, got: %s", errMsg)
	}
	if p.Name != "openai" {
		t.Errorf("Name = %q, want openai", p.Name)
	}
	if p.APIKey != "sk-123" {
		t.Errorf("APIKey = %q, want sk-123", p.APIKey)
	}
	if p.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (unset)", p.BaseURL)
	}
}

func TestProviderFormEditPreFill(t *testing.T) {
	orig := config.AIProvider{
		Name:    "work",
		APIKey:  "sk-original",
		BaseURL: "https://internal.corp/v1",
	}
	f := NewProviderForm()
	f.ShowEdit(orig)

	if f.mode != formModeEdit {
		t.Error("expected formModeEdit")
	}
	if f.editName != "work" {
		t.Errorf("editName = %q, want work", f.editName)
	}
	if got := f.fields[pfName].Value(); got != "work" {
		t.Errorf("name = %q, want work", got)
	}
	if got := f.fields[pfKey].Value(); got != "sk-original" {
		t.Errorf("key = %q, want sk-original", got)
	}
	if got := f.fields[pfBaseURL].Value(); got != "https://internal.corp/v1" {
		t.Errorf("base url = %q", got)
	}
	// A plaintext key implies "plain" mode.
	if got := f.secretsMode(); got != "plain" {
		t.Errorf("secretsMode = %q, want plain", got)
	}
}

// A keychain reference in the config infers "keychain" mode on edit.
func TestProviderFormSecretsModeFromReference(t *testing.T) {
	orig := config.AIProvider{
		Name:   "kced",
		APIKey: "secret://ai/kced/api_key",
	}
	f := NewProviderForm()
	f.ShowEdit(orig)
	if got := f.secretsMode(); got != "keychain" {
		t.Errorf("secretsMode for a ref = %q, want keychain", got)
	}
}

// secretsMode normalizes blank/typos to the documented default.
func TestProviderFormSecretsModeNormalization(t *testing.T) {
	cases := map[string]string{
		"":         "keychain",
		"keychain": "keychain",
		"KEY":      "keychain",
		"plain":    "plain",
		"garbage":  "plain",
	}
	for in, want := range cases {
		f := NewProviderForm()
		f.Show()
		f.fields[pfSecrets].SetValue(in)
		if got := f.secretsMode(); got != want {
			t.Errorf("secretsMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// Cycling the Secrets selector with h/l moves between keychain and plain.
func TestProviderFormCycleSecrets(t *testing.T) {
	f := NewProviderForm()
	f.Show()
	if got := f.fields[pfSecrets].Value(); got != "keychain" {
		t.Fatalf("initial = %q, want keychain", got)
	}
	f.cycleChoice(pfSecrets, 1)
	if got := f.fields[pfSecrets].Value(); got != "plain" {
		t.Errorf("after cycle = %q, want plain", got)
	}
	// Wraps back to keychain.
	f.cycleChoice(pfSecrets, 1)
	if got := f.fields[pfSecrets].Value(); got != "keychain" {
		t.Errorf("after wrap = %q, want keychain", got)
	}
}

// Show clears any stale state from a previous (e.g. edit) session.
func TestProviderFormShowResetsState(t *testing.T) {
	f := NewProviderForm()
	f.ShowEdit(config.AIProvider{Name: "old", APIKey: "k", BaseURL: "u"})
	f.SetError("stale")
	f.testing = true

	f.Show()
	if f.editName != "" {
		t.Errorf("editName = %q, want empty after Show", f.editName)
	}
	if f.mode != formModeAdd {
		t.Error("expected formModeAdd after Show")
	}
	if f.errMsg != "" {
		t.Errorf("errMsg = %q, want empty after Show", f.errMsg)
	}
	if f.testing {
		t.Error("testing flag should be cleared on Show")
	}
	if got := f.fields[pfKey].Value(); got != "" {
		t.Errorf("key field = %q, want empty after Show", got)
	}
}

// --- test-connection classification ----------------------------------------

func TestProviderFormClassifySuccess(t *testing.T) {
	f := NewProviderForm()
	st := f.classifyTestError(nil)
	if st[pfKey] != testOK || st[pfBaseURL] != testOK {
		t.Errorf("success states = %+v, want both testOK", st)
	}
}

func TestProviderFormClassifyAuthError(t *testing.T) {
	f := NewProviderForm()
	st := f.classifyTestError(errStringer("401 Unauthorized: invalid api key"))
	if st[pfKey] != testFail {
		t.Errorf("auth error should flag the key, got %+v", st)
	}
	if _, ok := st[pfBaseURL]; ok {
		t.Errorf("base url should be untouched on auth error, got %+v", st)
	}
}

func TestProviderFormClassifyEndpointError(t *testing.T) {
	f := NewProviderForm()
	st := f.classifyTestError(errStringer("dial tcp: lookup api.example.com: no such host"))
	if st[pfBaseURL] != testFail {
		t.Errorf("dial/dns error should flag the base url, got %+v", st)
	}
}

func TestProviderFormClassifyUnknownErrorNeutral(t *testing.T) {
	f := NewProviderForm()
	st := f.classifyTestError(errStringer("something unexpected happened"))
	if len(st) != 0 {
		t.Errorf("unknown error should be neutral (empty map), got %+v", st)
	}
}

// errStringer is a tiny error wrapper so classifyTestError has a real error
// value to switch on (instead of errors.New, which would pull fmt into the
// test file).
type errStringer string

func (e errStringer) Error() string { return string(e) }
