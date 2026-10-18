package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
	"pack_mate/internal/domain"
)

// ResetPasswordInput contains authenticated password reset parameters.
type ResetPasswordInput struct {
	UserID      string
	Phone       string
	Code        string
	NewPassword string
}

// ResetPassword replaces an existing password after verifying the bound phone number.
func (s *authService) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	userID, err := parseObjectID(input.UserID)
	if err != nil {
		return fmt.Errorf("invalid input")
	}
	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return fmt.Errorf("phone code is required")
	}
	if len(code) != 6 || !pnvsDigits(code) {
		return fmt.Errorf("phone code is invalid")
	}
	if input.NewPassword == "" {
		return fmt.Errorf("new password is required")
	}
	if err := validatePasswordStrength(input.NewPassword); err != nil {
		return err
	}
	if _, err := s.getActiveUserByID(ctx, userID); err != nil {
		return err
	}
	identity, err := s.identities.GetByUserIDAndProvider(ctx, userID, domain.AuthProviderPhone)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("phone does not match current user")
	}
	if err != nil {
		return fmt.Errorf("get phone identity failed: %w", err)
	}
	if identity.UserID != userID || identity.Identifier != phone || identity.Status != domain.AuthIdentityStatusActive {
		return fmt.Errorf("phone does not match current user")
	}
	if s.passwords == nil {
		return fmt.Errorf("password credential repository is not configured")
	}
	credential, err := s.passwords.GetByUserID(ctx, userID)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("password credential not found")
	}
	if err != nil {
		return fmt.Errorf("get password credential failed: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(input.NewPassword)) == nil {
		return fmt.Errorf("new password must be different")
	}
	hash, err := s.hashPassword(input.NewPassword)
	if err != nil {
		return fmt.Errorf("hash password failed: %w", err)
	}
	if s.env == "dev" {
		if code != s.devFixedCode {
			return fmt.Errorf("phone code is invalid")
		}
	} else {
		if s.env != "prod" || s.verification == nil {
			return &phoneVerificationError{public: ErrPhoneVerificationUnavailable, cause: fmt.Errorf("phone verification service is not configured")}
		}
		if err := s.verifyPhoneCode(ctx, phone, code); err != nil {
			return err
		}
	}
	if err := s.passwords.UpdatePasswordAndRevokeTokens(ctx, userID, credential.PasswordHash, hash, passwordAlgorithmBcrypt, time.Now().UTC()); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ErrPasswordChanged
		}
		return fmt.Errorf("reset password failed: %w", err)
	}
	return nil
}

// ErrPasswordChanged indicates that the password changed during verification.
var ErrPasswordChanged = errors.New("password changed; retry password reset")
