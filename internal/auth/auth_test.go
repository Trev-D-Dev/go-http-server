package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateJWT(t *testing.T) {
	// First, create some test data
	userID := uuid.New()
	secret := "test-secret"

	// Create a valid token for testing
	validToken, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test token: %v", err)
	}

	// Define test cases
	tests := []struct {
		name        string
		tokenString string
		tokenSecret string
		wantUserID  uuid.UUID
		wantErr     bool
	}{
		{
			name:        "Valid token",
			tokenString: validToken,
			tokenSecret: secret,
			wantUserID:  userID,
			wantErr:     false,
		},
		{
			name:        "Wrong secret",
			tokenString: validToken,
			tokenSecret: "wrong-secret",
			wantUserID:  uuid.Nil, // Zero value for UUID
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUserID, err := ValidateJWT(tt.tokenString, tt.tokenSecret)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJWT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotUserID != tt.wantUserID {
				t.Errorf("ValidateJWT() = %v, want %v", gotUserID, tt.wantUserID)
			}
		})
	}
}

func TestValidateJWT_ExpiredToken(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	// Create a token that expires very quickly
	expiredToken, err := MakeJWT(userID, secret, time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create test token: %v", err)
	}

	// Wait for it to expire
	time.Sleep(10 * time.Millisecond)

	// Try to validate the expired token
	_, err = ValidateJWT(expiredToken, secret)
	if err == nil {
		t.Error("Expected error for expired token, but got none")
	}
}
