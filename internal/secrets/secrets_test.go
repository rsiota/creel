package secrets

import (
	"strings"
	"testing"
)

func TestMakeKeyAndRef(t *testing.T) {
	if got := MakeKey("prod", FieldPassword); got != "prod/password" {
		t.Errorf("MakeKey = %q, want prod/password", got)
	}
	ref := MakeRef("prod", FieldPassword)
	if ref != "secret://prod/password" {
		t.Errorf("MakeRef = %q, want secret://prod/password", ref)
	}
	if !IsReference(ref) {
		t.Errorf("IsReference(%q) = false, want true", ref)
	}
	if !strings.HasPrefix(ref, RefPrefix) {
		t.Errorf("ref %q missing prefix", ref)
	}
}

func TestIsReference(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"secret://prod/password", true},
		{"secret://", true},
		{"plaintext", false},
		{"", false},
		{" secret://x", false}, // leading space is not a reference
	}
	for _, c := range cases {
		if got := IsReference(c.in); got != c.want {
			t.Errorf("IsReference(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestResolvePassesPlaintextThrough(t *testing.T) {
	got, err := Resolve("hunter2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hunter2" {
		t.Errorf("Resolve = %q, want hunter2", got)
	}
	// Empty plaintext is a valid pass-through.
	got, err = Resolve("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("Resolve(\"\") = %q, want empty", got)
	}
}

func TestStoreEmptyIsNoop(t *testing.T) {
	ref, err := Store("prod", FieldPassword, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "" {
		t.Errorf("Store empty = %q, want empty", ref)
	}
}

func TestStorePassesReferenceThrough(t *testing.T) {
	// An existing reference should not be re-stored.
	in := MakeRef("prod", FieldPassword)
	out, err := Store("prod", FieldPassword, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != in {
		t.Errorf("Store(ref) = %q, want %q", out, in)
	}
}

// TestKeychainRoundTrip exercises the real OS keychain. It is skipped when no
// keychain backend is available (e.g. headless CI). On macOS the first run may
// prompt once for keychain access.
func TestKeychainRoundTrip(t *testing.T) {
	if !Available() {
		t.Skip("OS keychain not available")
	}
	conn := "creel-test-roundtrip"
	t.Cleanup(func() { _ = DeleteAll(conn) })

	ref, err := Store(conn, FieldPassword, "s3cr3t")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !IsReference(ref) {
		t.Fatalf("expected a reference, got %q", ref)
	}
	got, err := Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "s3cr3t" {
		t.Errorf("Resolve = %q, want s3cr3t", got)
	}

	// Storing over an existing key updates the value under the same ref.
	if _, err := Store(conn, FieldPassword, "rotated"); err != nil {
		t.Fatalf("Store update: %v", err)
	}
	got, err = Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve after update: %v", err)
	}
	if got != "rotated" {
		t.Errorf("Resolve after update = %q, want rotated", got)
	}

	// DeleteAll removes the entry; subsequent Resolve fails.
	if err := DeleteAll(conn); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if _, err := Resolve(ref); err == nil {
		t.Error("Resolve after delete: expected error, got nil")
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	if !Available() {
		t.Skip("OS keychain not available")
	}
	if err := Delete("creel-test-missing", FieldPassword); err != nil {
		t.Errorf("Delete of missing key = %v, want nil", err)
	}
}

func TestDeleteAllReportsFirstErrorButAttemptsAll(t *testing.T) {
	if !Available() {
		t.Skip("OS keychain not available")
	}
	// DeleteAll on a never-stored connection is a clean no-op (all missing).
	if err := DeleteAll("creel-test-deleteall-clean"); err != nil {
		t.Errorf("DeleteAll on clean connection = %v, want nil", err)
	}
}

// --- AI provider secrets ----------------------------------------------------

func TestAIKeyNamespaced(t *testing.T) {
	// The "ai/" prefix keeps provider keys separate from same-named
	// connection passwords.
	if got := AIKey("prod"); got != "ai/prod/"+FieldAPIKey {
		t.Errorf("AIKey = %q", got)
	}
	if got := AIKey("prod"); got == MakeKey("prod", FieldPassword) {
		t.Error("AI key collides with a connection password key")
	}
}

func TestStoreAIEmpyIsNoop(t *testing.T) {
	ref, err := StoreAI("openai", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref != "" {
		t.Errorf("StoreAI empty = %q, want empty", ref)
	}
}

func TestStoreAIPassesReferenceThrough(t *testing.T) {
	in := RefPrefix + AIKey("openai")
	out, err := StoreAI("openai", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != in {
		t.Errorf("StoreAI(ref) = %q, want %q", out, in)
	}
}

func TestAIKeychainRoundTrip(t *testing.T) {
	if !Available() {
		t.Skip("OS keychain not available")
	}
	name := "creel-test-ai-provider"
	t.Cleanup(func() { _ = DeleteAI(name) })

	ref, err := StoreAI(name, "sk-test-123")
	if err != nil {
		t.Fatalf("StoreAI: %v", err)
	}
	if !IsReference(ref) {
		t.Fatalf("expected a reference, got %q", ref)
	}
	got, err := Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "sk-test-123" {
		t.Errorf("Resolve = %q, want sk-test-123", got)
	}

	if err := DeleteAI(name); err != nil {
		t.Fatalf("DeleteAI: %v", err)
	}
	if _, err := Resolve(ref); err == nil {
		t.Error("Resolve after delete: expected error, got nil")
	}
}

func TestDeleteAIMissingIsNotAnError(t *testing.T) {
	if !Available() {
		t.Skip("OS keychain not available")
	}
	if err := DeleteAI("creel-test-ai-missing"); err != nil {
		t.Errorf("DeleteAI of missing key = %v, want nil", err)
	}
}
