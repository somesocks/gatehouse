package keychain

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestKDFRoundTrip(t *testing.T) {
	salt := []byte("0123456789abcdef")
	value := KDF{Salt: salt}.String()
	want := "gh-kdf:MDEyMzQ1Njc4OWFiY2RlZg?alg=pbkdf2-hmac-sha256-v1"
	if value != want {
		t.Fatalf("KDF.String() = %q, want %q", value, want)
	}
	err, parsed := ParseKDF(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed.Salt) != string(salt) {
		t.Fatalf("ParseKDF() salt = %x, want %x", parsed.Salt, salt)
	}
}

func TestParseKDFRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		value string
	}{
		{"gh-kdf:?alg=pbkdf2-hmac-sha256-v1"},
		{"gh-kdf:MDEyMzQ1Njc4OWFiY2RlZg?alg=argon2id-v1"},
		{"gh-kdf:MDEyMzQ1Njc4OWFiY2RlZg?extra=value&alg=pbkdf2-hmac-sha256-v1"},
		{"gh-kdf:MDEyMzQ1Njc4OWFiY2RlZg?alg=pbkdf2-hmac-sha256-v1#fragment"},
		{"gh-kdf:short?alg=pbkdf2-hmac-sha256-v1"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			err, _ := ParseKDF(test.value)
			if err == nil {
				t.Fatalf("ParseKDF(%q) succeeded", test.value)
			}
		})
	}
}

func TestEncryptedRoundTrip(t *testing.T) {
	payload := []byte("0123456789abcdefghijklmnopqr")
	value := Encrypted{Payload: payload, Key: &KeychainRef{ID: "default", Version: 1}}.String()
	want := "gh-enc:" + base64.RawURLEncoding.EncodeToString(payload) + "?alg=aes128-gcm-v1&key=default&ver=1"
	if value != want {
		t.Fatalf("Encrypted.String() = %q, want %q", value, want)
	}
	err, parsed := ParseResource(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed.Payload) != string(payload) || parsed.Key == nil || *parsed.Key != (KeychainRef{ID: "default", Version: 1}) {
		t.Fatalf("ParseResource() = %#v, want payload %x and default/1", parsed, payload)
	}
}

func TestParseEncryptedRejectsInvalidValues(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdefghijklmnopqr"))
	tests := []struct {
		name     string
		value    string
		parse    func(string) (error, Encrypted)
		contains string
	}{
		{"missing algorithm", "gh-enc:" + payload, ParseEncrypted, "requires alg"},
		{"unknown algorithm", "gh-enc:" + payload + "?alg=aes256-gcm-v1", ParseEncrypted, "requires alg"},
		{"noncanonical query", "gh-enc:" + payload + "?key=default&alg=aes128-gcm-v1&ver=1", ParseEncrypted, "query must be canonical"},
		{"missing key version", "gh-enc:" + payload + "?alg=aes128-gcm-v1&key=default", ParseEncrypted, "specified together"},
		{"invalid key", "gh-enc:" + payload + "?alg=aes128-gcm-v1&key=Default&ver=1", ParseEncrypted, "key must match"},
		{"noncanonical version", "gh-enc:" + payload + "?alg=aes128-gcm-v1&key=default&ver=01", ParseEncrypted, "canonical"},
		{"keychain key has selector", "gh-enc:" + payload + "?alg=aes128-gcm-v1&key=default&ver=1", ParseKey, "must not select"},
		{"resource lacks selector", "gh-enc:" + payload + "?alg=aes128-gcm-v1", ParseResource, "requires key"},
		{"short payload", "gh-enc:" + base64.RawURLEncoding.EncodeToString([]byte("short")) + "?alg=aes128-gcm-v1", ParseEncrypted, "authentication tag"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, _ := test.parse(test.value)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("parse(%q) error = %v, want %q", test.value, err, test.contains)
			}
		})
	}
}
