// Package password hashes and verifies user passwords with bcrypt.
package password

import "golang.org/x/crypto/bcrypt"

// Cost is the bcrypt work factor used for new hashes. Hashes created with a
// different cost keep verifying: bcrypt stores the cost inside the hash.
const Cost = 12

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), Cost)
	return string(hashedPassword), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
