package main

// Unit tests af password-funktionerne alene, uden database eller HTTP

import (
	"strings"
	"testing"
)

func TestCheckPasswordAcceptsOnlyTheRightPassword(t *testing.T) {
	hash, err := hashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(hash, "correct-horse") {
		t.Error("the right password was rejected")
	}
	if checkPassword(hash, "wrong-horse") {
		t.Error("a wrong password was accepted")
	}
}

func TestHashPasswordUsesSalt(t *testing.T) {
	first, err := hashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	// Samme password skal give forskellige hashes, ellers kan de slås op i en tabel
	if first == second {
		t.Error("two hashes of the same password are identical, so no salt is used")
	}
}

func TestPasswordsLongerThan72BytesStillWork(t *testing.T) {
	long := strings.Repeat("a", 100)
	hash, err := hashPassword(long)
	if err != nil {
		t.Fatalf("bcrypt rejects passwords over 72 bytes unless they are shortened: %v", err)
	}
	if !checkPassword(hash, long) {
		t.Error("a long password could not log in again")
	}
}
