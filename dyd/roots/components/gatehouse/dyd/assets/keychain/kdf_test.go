package keychain

import (
	"bytes"
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

func TestDeriveKEKRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		passphrase []byte
		kdf        KDF
	}{
		{nil, KDF{Salt: []byte("0123456789abcdef")}},
		{[]byte("passphrase"), KDF{Salt: []byte("short")}},
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
