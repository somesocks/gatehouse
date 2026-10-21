package keychain

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestDeriveKEKIsDeterministicAndSalted(t *testing.T) {
	passphrase := []byte("correct horse battery staple")
	first := KDF{Salt: []byte("0123456789abcdef")}
	second := KDF{Salt: []byte("fedcba9876543210")}

	err, firstKey := DeriveKEK(passphrase, first)
	if err != nil {
		t.Fatal(err)
	}
	err, repeatedKey := DeriveKEK(passphrase, first)
	if err != nil {
		t.Fatal(err)
	}
	err, secondKey := DeriveKEK(passphrase, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstKey) != keySize {
		t.Fatalf("DeriveKEK() key length = %d, want %d", len(firstKey), keySize)
	}
	if !bytes.Equal(firstKey, repeatedKey) {
		t.Fatal("DeriveKEK() changed for identical input")
	}
	if bytes.Equal(firstKey, secondKey) {
		t.Fatal("DeriveKEK() did not vary with salt")
	}
}

func TestDeriveKEKAlgorithmVectors(t *testing.T) {
	for _, test := range []struct{ algorithm, want string }{
		{"", "6c4a646aad10d067add5fb79d9078a16"},
		{"pbkdf2-hmac-sha256-v1", "6c4a646aad10d067add5fb79d9078a16"},
		{"pbkdf2-hmac-sha256-v0", "b20410ffded3d9e108e29c53805b5d0a"},
	} {
		err, key := DeriveKEK([]byte("correct horse battery staple"), KDF{Salt: []byte("0123456789abcdef"), Algorithm: test.algorithm})
		if err != nil || hex.EncodeToString(key) != test.want {
			t.Fatalf("DeriveKEK(%q) = %x, %v; want %s", test.algorithm, key, err, test.want)
		}
	}
}

func TestDeriveKEKRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		passphrase []byte
		kdf        KDF
	}{
		{nil, KDF{Salt: []byte("0123456789abcdef")}},
		{[]byte("passphrase"), KDF{Salt: []byte("short")}},
		{[]byte("passphrase"), KDF{Salt: []byte("0123456789abcdef"), Algorithm: "unknown"}},
	}
	for _, test := range tests {
		t.Run("invalid", func(t *testing.T) {
			err, _ := DeriveKEK(test.passphrase, test.kdf)
			if err == nil {
				t.Fatal("DeriveKEK() succeeded for invalid input")
			}
		})
	}
}
