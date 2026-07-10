// Package secrets provides an OS keychain-backed secret store for connection
// credentials, with a transparent plaintext fallback.
//
// Secret values passed to Store are written to the OS keychain (via
// github.com/zalando/go-keyring) and represented in configuration files as
// opaque references of the form "secret://<key>". Resolve accepts either a
// reference or a plaintext value and always returns the plaintext. This keeps
// real passwords out of ~/.config/gsql/config.yaml while remaining backward
// compatible: any plaintext value is passed through unchanged.
//
// The keychain is optional. If the OS keychain is unavailable (headless Linux
// without D-Bus, a locked keychain, etc.), callers fall back to storing the
// plaintext value directly in the config file.
package secrets

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	// Service is the keychain service under which all gsql secrets are stored.
	Service = "gsql"
	// RefPrefix marks a configuration value as a keychain reference. Values
	// starting with this prefix are resolved via Resolve rather than stored.
	RefPrefix = "secret://"
)

// Per-field keys used to build stable, connection-scoped keychain entries.
const (
	FieldPassword      = "password"
	FieldSSHPassword   = "ssh_password"
	FieldSSHPassphrase = "ssh_passphrase"
)

// Fields is the ordered set of connection-config fields that may hold secrets.
// Callers iterate this to resolve or store every secret field uniformly.
var Fields = []string{FieldPassword, FieldSSHPassword, FieldSSHPassphrase}

// availabilityProbeKey is a sentinel used by Available to detect a functional
// keychain backend without reading or writing real data.
const availabilityProbeKey = "__gsql_availability_probe__"

// MakeKey builds the keychain key for a connection's field. The key is stable
// across runs as long as the connection name is unchanged.
func MakeKey(connName, field string) string {
	return connName + "/" + field
}

// MakeRef builds the config-file representation of a stored secret.
func MakeRef(connName, field string) string {
	return RefPrefix + MakeKey(connName, field)
}

// IsReference reports whether v is a keychain reference produced by Store.
func IsReference(v string) bool {
	return strings.HasPrefix(v, RefPrefix)
}

// Available reports whether the OS keychain is usable. When false, callers
// should keep secrets as plaintext rather than calling Store.
//
// It probes a sentinel key: a functional backend returns keyring.ErrNotFound
// for a missing key, whereas an unavailable backend returns a different error
// (or one of the platform-specific "not implemented" errors).
func Available() bool {
	_, err := keyring.Get(Service, availabilityProbeKey)
	return err == nil || errors.Is(err, keyring.ErrNotFound)
}

// Store writes value to the OS keychain and returns a reference string for
// config storage. Empty values return "" without touching the keychain. Values
// that are already references are returned unchanged. If the keychain is
// unavailable, Store returns an error so the caller can fall back to plaintext.
func Store(connName, field, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if IsReference(value) {
		return value, nil
	}
	key := MakeKey(connName, field)
	if err := keyring.Set(Service, key, value); err != nil {
		return "", fmt.Errorf("storing %q in keychain: %w", key, err)
	}
	return MakeRef(connName, field), nil
}

// Resolve returns the plaintext form of value. Values that are not references
// are returned unchanged. A reference whose secret is missing from the keychain
// yields an error.
func Resolve(value string) (string, error) {
	if !IsReference(value) {
		return value, nil
	}
	key := strings.TrimPrefix(value, RefPrefix)
	secret, err := keyring.Get(Service, key)
	if err != nil {
		return "", fmt.Errorf("reading %q from keychain: %w", key, err)
	}
	return secret, nil
}

// Delete removes a single connection field from the keychain. A missing key is
// not an error (the connection may never have used the keychain). Other
// failures are returned.
func Delete(connName, field string) error {
	key := MakeKey(connName, field)
	err := keyring.Delete(Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deleting %q from keychain: %w", key, err)
	}
	return nil
}

// DeleteAll removes every known secret field for a connection. Each field is
// attempted even if an earlier one fails; the first error encountered is
// returned. Safe to call for a connection that never used the keychain.
func DeleteAll(connName string) error {
	var first error
	for _, field := range Fields {
		if err := Delete(connName, field); err != nil && first == nil {
			first = err
		}
	}
	return first
}
