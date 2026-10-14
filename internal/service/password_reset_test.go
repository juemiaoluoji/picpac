package service

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
	"pack_mate/internal/domain"
	"testing"
	"time"
)

// UpdatePasswordAndRevokeTokens simulates the conditional atomic repository operation.
func (r *fakeUserPasswordCredentialRepository) UpdatePasswordAndRevokeTokens(ctx context.Context, id bson.ObjectID, expectedHash, hash, algo string, at time.Time) error {
	if r.got == nil || r.got.PasswordHash != expectedHash {
		return mongo.ErrNoDocuments
	}
	return r.UpdatePassword(ctx, id, hash, algo, at)
}

// TestResetPassword verifies shared business rules and environment-specific verification.
func TestResetPassword(t *testing.T) {
	for _, name := range []string{"dev", "prod", "wrong code", "provider failure", "wrong phone", "weak password", "same password", "missing credential", "database failure", "concurrent change", "invalid user", "invalid code", "unknown environment", "disabled user", "inactive identity", "dev wrong code", "dev custom code"} {
		t.Run(name, func(t *testing.T) {
			id := bson.NewObjectID()
			old, _ := bcrypt.GenerateFromPassword([]byte("OldPass2026!"), bcrypt.MinCost)
			users := &fakeUserRepository{got: &domain.User{ID: id, Status: domain.UserStatusCreated}}
			identities := &fakeAuthIdentityRepository{got: &domain.AuthIdentity{UserID: id, Identifier: "+8613800138000", Provider: domain.AuthProviderPhone, Status: domain.AuthIdentityStatusActive}}
			passwords := &fakeUserPasswordCredentialRepository{got: &domain.UserPasswordCredential{UserID: id, PasswordHash: string(old)}}
			provider := &recordingPhoneVerificationService{}
			cfg := validAuthConfig()
			cfg.Password.BcryptCost = bcrypt.MinCost
			env := "prod"
			if name == "dev" || name == "dev wrong code" || name == "dev custom code" {
				env = "dev"
			}
			input := ResetPasswordInput{UserID: id.Hex(), Phone: "13800138000", Code: "123456", NewPassword: "NewPass2026!"}
			switch name {
			case "disabled user":
				users.got.Status = domain.UserStatusDisabled
			case "inactive identity":
				identities.got.Status = domain.AuthIdentityStatusDisabled
			case "dev wrong code":
				input.Code = "000000"
			case "dev custom code":
				cfg.PhoneCode.DevFixedCode = "654321"
				input.Code = "654321"
			case "wrong code":
				input.Code = "000000"
			case "provider failure":
				provider.verifyErr = errors.New("unavailable")
			case "wrong phone":
				input.Phone = "13900139000"
			case "weak password":
				input.NewPassword = "a"
			case "same password":
				input.NewPassword = "OldPass2026!"
			case "missing credential":
				passwords.got = nil
			case "database failure":
				passwords.updateErr = errors.New("transaction failed")
			case "concurrent change":
				passwords.updateErr = mongo.ErrNoDocuments
			case "invalid user":
				input.UserID = "bad"
			case "invalid code":
				input.Code = "１２３４５６"
			case "unknown environment":
				env = "test"
			}
			svc := NewAuthService(users, identities, &fakeRefreshTokenRepository{}, passwords, &fakeAuthUploadService{}, provider, NewTokenService(cfg.AccessTokenSecret, time.Hour), cfg, env)
			err := svc.ResetPassword(context.Background(), input)
			success := name == "dev" || name == "prod" || name == "dev custom code"
			if (err == nil) != success {
				t.Fatalf("unexpected error: %v", err)
			}
			if success {
				if bcrypt.CompareHashAndPassword([]byte(passwords.updatedPasswordHash), []byte(input.NewPassword)) != nil {
					t.Fatal("new password not stored")
				}
			} else if passwords.updatedPasswordHash != "" {
				t.Fatal("password changed on failure")
			}
			if env == "dev" && provider.verifyCalls != 0 {
				t.Fatal("dev contacted provider")
			}
			if name == "prod" && (provider.verifyCalls != 1 || provider.phone != "+8613800138000") {
				t.Fatal("prod did not verify normalized phone")
			}
			if name == "provider failure" && !errors.Is(err, ErrPhoneVerificationUnavailable) {
				t.Fatal("provider error not classified")
			}
			if name == "concurrent change" && !errors.Is(err, ErrPasswordChanged) {
				t.Fatal("conflict not classified")
			}
		})
	}
}
