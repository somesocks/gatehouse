package keychain

import (
	"bytes"
	"strings"
	"testing"
)

func TestSealAndOpen(t *testing.T) {
	key := []byte("0123456789abcdef")
	random := bytes.NewReader([]byte("0123456789ab"))
	associatedData := []byte("gatehouse:v1|workspace=engineering|resource=token")
	plaintext := []byte("super secret value")

	err, encrypted := Seal(random, key, associatedData, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := encrypted.Payload[:12], []byte("0123456789ab"); !bytes.Equal(got, want) {
		t.Fatalf("Seal() nonce = %x, want %x", got, want)
	}
	err, result := Open(key, associatedData, encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, plaintext) {
		t.Fatalf("Open() = %q, want %q", result, plaintext)
	}
}

func TestOpenRejectsWrongKeyPayloadAndAssociatedData(t *testing.T) {
	key := []byte("0123456789abcdef")
	err, encrypted := Seal(bytes.NewReader([]byte("0123456789ab")), key, []byte("first"), []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name           string
		key            []byte
		associatedData []byte
		encrypted      Encrypted
	}{
		{"wrong key", []byte("fedcba9876543210"), []byte("first"), encrypted},
		{"wrong associated data", key, []byte("second"), encrypted},
		{
			"modified payload",
			key,
			[]byte("first"),
			Encrypted{Payload: append([]byte(nil), encrypted.Payload...)},
		},
	}
	tests[2].encrypted.Payload[len(tests[2].encrypted.Payload)-1] ^= 1
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, _ := Open(test.key, test.associatedData, test.encrypted)
			if err == nil || !strings.Contains(err.Error(), "decrypt encrypted payload") {
				t.Fatalf("Open() error = %v, want authentication failure", err)
			}
		})
	}
}

func TestSealAndOpenRejectInvalidKeys(t *testing.T) {
	for _, key := range [][]byte{nil, []byte("short"), []byte("0123456789abcdef0")} {
		t.Run("invalid", func(t *testing.T) {
			err, _ := Seal(bytes.NewReader([]byte("0123456789ab")), key, nil, nil)
			if err == nil || !strings.Contains(err.Error(), "requires a 16-byte key") {
				t.Fatalf("Seal() error = %v, want invalid key error", err)
			}
		})
	}
}
