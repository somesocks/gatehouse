package identity

import "testing"

func TestPasswordVerifierVerifiesOnlyItsPassword(t *testing.T) {
	value, err := passwordVerifier([]byte("correct horse battery staple"))
	if err != nil {
		t.Fatal(err)
	}
	verifier := value.(string)
	if err, valid := VerifyPassword(verifier, []byte("correct horse battery staple")); err != nil || !valid {
		t.Fatalf("VerifyPassword() = (%v, %t), want (nil, true)", err, valid)
	}
	if err, valid := VerifyPassword(verifier, []byte("wrong password")); err != nil || valid {
		t.Fatalf("VerifyPassword() = (%v, %t), want (nil, false)", err, valid)
	}
}
