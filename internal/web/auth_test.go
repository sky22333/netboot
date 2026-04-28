package web

import "testing"

func TestPasswordHashVerify(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if verifyPassword(hash, "wrong password") {
		t.Fatal("wrong password verified")
	}
}
