package identity

import (
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/kdf"
)

func TestPasswordVerifierVerifiesOnlyItsPassword(t *testing.T) {
	value, err := passwordVerifier([]byte("correct horse battery staple"), config.Crypto{})
	if err != nil {
		t.Fatal(err)
	}
	verifier := value
	if err, valid := VerifyPassword(verifier, []byte("correct horse battery staple")); err != nil || !valid {
		t.Fatalf("VerifyPassword() = (%v, %t), want (nil, true)", err, valid)
	}
	if err, valid := VerifyPassword(verifier, []byte("wrong password")); err != nil || valid {
		t.Fatalf("VerifyPassword() = (%v, %t), want (nil, false)", err, valid)
	}
}

func TestPasswordVerifierAlgorithms(t *testing.T) {
	const password = "correct horse battery staple"
	// Fixed vectors also cover existing v1 records independently of generation.
	for _, vector := range []string{
		"gh-ver:MDEyMzQ1Njc4OWFiY2RlZg.sgQQ_97T2eEI4pxTgFtdCgvoTQ_pAWB0KRhU1WpOw8Y?alg=pbkdf2-hmac-sha256-v0",
		"gh-ver:MDEyMzQ1Njc4OWFiY2RlZg.bEpkaq0Q0Get1ft52QeKFtqD1Q-BZwqOdZOySebZSTY?alg=pbkdf2-hmac-sha256-v1",
	} {
		if err, valid := VerifyPassword(vector, []byte(password)); err != nil || !valid {
			t.Fatalf("VerifyPassword(%q) = %v, %t", vector, err, valid)
		}
		if err, valid := VerifyPassword(vector, []byte("wrong")); err != nil || valid {
			t.Fatalf("wrong password = %v, %t", err, valid)
		}
		for _, query := range []string{"alg=unknown", "alg=", "alg=pbkdf2-hmac-sha256-v0&extra=1", "alg=pbkdf2-hmac-sha256-v0&alg=pbkdf2-hmac-sha256-v1"} {
			invalid := strings.Split(vector, "?")[0] + "?" + query
			if err, valid := VerifyPassword(invalid, []byte(password)); err == nil || valid {
				t.Fatalf("invalid algorithm accepted: %q", invalid)
			}
		}
	}
	for _, algorithm := range []string{kdf.InsecureV0, kdf.Default} {
		crypto := config.Crypto{DefaultKDF: algorithm}
		value, err := NewPasswordVerifier([]byte(password), crypto)
		if err != nil || !strings.HasSuffix(value, "?alg="+algorithm) {
			t.Fatalf("NewPasswordVerifier() = %q, %v", value, err)
		}
		if err, valid := VerifyPassword(value, []byte(password)); err != nil || !valid {
			t.Fatalf("generated verifier = %v, %t", err, valid)
		}
		err, dummy := NewDummyPasswordVerifier(crypto)
		if err != nil || !strings.HasSuffix(dummy, "?alg="+algorithm) {
			t.Fatalf("dummy verifier = %q, %v", dummy, err)
		}
	}
	if _, err := NewPasswordVerifier([]byte(password), config.Crypto{DefaultKDF: "unknown"}); err == nil {
		t.Fatal("unknown creation algorithm accepted")
	}
}
