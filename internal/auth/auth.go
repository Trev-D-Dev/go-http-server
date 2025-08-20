package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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

func CheckPasswordHash(hashedPassword, password string) error {

	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		returnErr := fmt.Errorf("error checking hash: %w", err)
		return returnErr
	}

	return nil
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	})

	signedJwt, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		returnErr := fmt.Errorf("error signing token: %w", err)
		return "", returnErr
	}

	return signedJwt, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(tokenSecret), nil
	})
	if err != nil {
		returnErr := fmt.Errorf("error parsing token: %w", err)
		return uuid.UUID{}, returnErr
	}

	tokenClaims := token.Claims

	subjectString, err := tokenClaims.GetSubject()
	if err != nil {
		returnErr := fmt.Errorf("error retrieving token subject: %v", err)
		return uuid.UUID{}, returnErr
	}

	userID, err := uuid.Parse(subjectString)
	if err != nil {
		returnErr := fmt.Errorf("error parsing uuid for token: %v", err)
		return uuid.UUID{}, returnErr
	}

	return userID, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authSlice, ok := headers["Authorization"]
	if !ok {
		returnErr := fmt.Errorf("error retrieving authorization header")
		return "", returnErr
	}

	authString := authSlice[0]

	tokenString := strings.Trim(authString, " ")

	spcIdx := strings.Index(tokenString, " ")
	if spcIdx == -1 {
		returnErr := fmt.Errorf("error retrieving token")
		return "", returnErr
	}

	tokenString = tokenString[(spcIdx + 1):]

	tokenString = strings.Trim(tokenString, " ")

	return tokenString, nil
}
