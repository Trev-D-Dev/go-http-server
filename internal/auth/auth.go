package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hashPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		returnErr := fmt.Errorf("error hashing password %w", err)
		return "", returnErr
	}

	hashString := string(hashPass)

	return hashString, nil
}

func CheckPasswordHash(password, hash string) error {

	err := bcrypt.CompareHashAndPassword([]byte(password), []byte(hash))
	if err != nil {
		returnErr := fmt.Errorf("error checking hash: %w", err)
		return returnErr
	}

	return nil
}
