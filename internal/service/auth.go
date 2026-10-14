package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"pack_mate/internal/config"
	"pack_mate/internal/domain"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/repository"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"
)

// AuthResult contains the authenticated user and tokens.
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	User         *domain.User
}

const (
	defaultUserAvatarObjectKey = "users/default/avatar.png"
	passwordAlgorithmBcrypt    = "bcrypt"
)

var (
	// ErrPhoneCodeRateLimited indicates that another code cannot be sent yet.
	ErrPhoneCodeRateLimited = errors.New("phone code send too frequently")
	// ErrPhoneVerificationUnavailable indicates that the external verification provider cannot complete the request.
	ErrPhoneVerificationUnavailable = errors.New("phone verification service is unavailable")
)

type phoneVerificationError struct {
	public error
	cause  error
}

// Error returns the stable client-facing message.
func (e *phoneVerificationError) Error() string {
	return e.public.Error()
}

// Unwrap preserves the provider failure for server-side diagnostics.
func (e *phoneVerificationError) Unwrap() error {
	return e.cause
}

// Is classifies the error by its stable public category.
func (e *phoneVerificationError) Is(target error) bool {
	return target == e.public
}

// RefreshResult contains a refreshed access token.
type RefreshResult struct {
	AccessToken string
}

// AuthService defines authentication behavior.
type AuthService interface {
	PatchMyProfile(ctx context.Context, userID string, input ProfilePatchInput) (*domain.User, error)
	ResetPassword(ctx context.Context, input ResetPasswordInput) error
	SendPhoneCode(ctx context.Context, input request.SendPhoneCodeInput) error
	LoginWithPhone(ctx context.Context, input request.PhoneLoginInput) (*AuthResult, error)
	LoginWithPhonePassword(ctx context.Context, input request.PhonePasswordLoginInput) (*AuthResult, error)
	Refresh(ctx context.Context, input request.RefreshTokenInput) (*RefreshResult, error)
	Logout(ctx context.Context, input request.LogoutInput) error
	SetupPassword(ctx context.Context, input request.SetupPasswordInput) error
	ChangePassword(ctx context.Context, input request.ChangePasswordInput) error
	GetSecurity(ctx context.Context, userID string) (*domain.AuthSecurity, error)
	Me(ctx context.Context, userID string) (*domain.User, error)
	UpdateMyProfile(ctx context.Context, userID string, input request.UpdateMyProfileInput) (*domain.User, error)
	DeleteMe(ctx context.Context, userID string) error
}

type authService struct {
	profileUpload   config.ProfileUploadConfig
	users           repository.UserRepository
	identities      repository.AuthIdentityRepository
	refreshTokens   repository.RefreshTokenRepository
	passwords       repository.UserPasswordCredentialRepository
	uploader        UploadService
	verification    PhoneVerificationService
	env             string
	devFixedCode    string
	tokens          TokenService
	passwordConfig  config.PasswordConfig
	refreshTokenTTL time.Duration
}

// NewAuthService creates an auth service.
func NewAuthService(
	users repository.UserRepository,
	identities repository.AuthIdentityRepository,
	refreshTokens repository.RefreshTokenRepository,
	passwords repository.UserPasswordCredentialRepository,
	uploader UploadService,
	verification PhoneVerificationService,
	tokens TokenService,
	authConfig config.AuthConfig,
	env string,
) AuthService {
	devFixedCode := strings.TrimSpace(authConfig.PhoneCode.DevFixedCode)
	if devFixedCode == "" {
		devFixedCode = "123456"
	}
	return &authService{
		profileUpload:   authConfig.ProfileUpload.WithDefaults(),
		users:           users,
		identities:      identities,
		refreshTokens:   refreshTokens,
		passwords:       passwords,
		uploader:        uploader,
		verification:    verification,
		env:             env,
		devFixedCode:    devFixedCode,
		tokens:          tokens,
		passwordConfig:  authConfig.Password,
		refreshTokenTTL: time.Duration(authConfig.RefreshTokenTTLSeconds) * time.Second,
	}
}

// SendPhoneCode sends a phone login code.
func (s *authService) SendPhoneCode(ctx context.Context, input request.SendPhoneCodeInput) error {
	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return err
	}
	if s.env == "dev" {
		return nil
	}
	if s.env != "prod" || s.verification == nil {
		return &phoneVerificationError{public: ErrPhoneVerificationUnavailable, cause: fmt.Errorf("phone verification service is not configured")}
	}

	if err := s.verification.SendCode(ctx, phone); err != nil {
		if errors.Is(err, errPhoneCodeRateLimited) {
			return &phoneVerificationError{public: ErrPhoneCodeRateLimited, cause: err}
		}
		return &phoneVerificationError{public: ErrPhoneVerificationUnavailable, cause: err}
	}

	return nil
}

// LoginWithPhone logs in with a phone code, creating a user on first login.
func (s *authService) LoginWithPhone(ctx context.Context, input request.PhoneLoginInput) (*AuthResult, error) {
	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(input.Code)
	if code == "" {
		return nil, fmt.Errorf("phone code is required")
	}
	if len(code) != 6 || !pnvsDigits(code) {
		return nil, fmt.Errorf("phone code is invalid")
	}
	if s.env == "dev" {
		if code != s.devFixedCode {
			return nil, fmt.Errorf("phone code is invalid")
		}
	} else {
		if s.env != "prod" || s.verification == nil {
			return nil, &phoneVerificationError{public: ErrPhoneVerificationUnavailable, cause: fmt.Errorf("phone verification service is not configured")}
		}
		if err := s.verifyPhoneCode(ctx, phone, code); err != nil {
			return nil, err
		}
	}

	user, err := s.getOrCreatePhoneUser(ctx, phone)
	if err != nil {
		return nil, err
	}
	return s.newAuthResult(ctx, user)
}

func (s *authService) verifyPhoneCode(ctx context.Context, phone, code string) error {
	valid, err := s.verification.VerifyCode(ctx, phone, code)
	if err != nil {
		return &phoneVerificationError{public: ErrPhoneVerificationUnavailable, cause: err}
	}
	if !valid {
		return fmt.Errorf("phone code is invalid")
	}
	return nil
}

// LoginWithPhonePassword logs in with a phone number and password.
func (s *authService) LoginWithPhonePassword(ctx context.Context, input request.PhonePasswordLoginInput) (*AuthResult, error) {
	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return nil, err
	}
	if input.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	identity, err := s.identities.GetByProviderAndIdentifier(ctx, domain.AuthProviderPhone, phone)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("phone or password is invalid")
		}
		return nil, fmt.Errorf("get auth identity failed: %w", err)
	}
	user, err := s.getActiveUserByID(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}

	if s.passwords == nil {
		return nil, fmt.Errorf("password credential repository is not configured")
	}
	credential, err := s.passwords.GetByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("phone or password is invalid")
		}
		return nil, fmt.Errorf("get password credential failed: %w", err)
	}

	now := time.Now().UTC()
	if credential.LockedUntil != nil && credential.LockedUntil.After(now) {
		return nil, fmt.Errorf("password is locked")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(input.Password)); err != nil {
		if recordErr := s.recordPasswordFailure(ctx, credential, now); recordErr != nil {
			return nil, recordErr
		}
		return nil, fmt.Errorf("phone or password is invalid")
	}
	if err := s.passwords.RecordSuccess(ctx, user.ID, now); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("phone or password is invalid")
		}
		return nil, fmt.Errorf("record password login success failed: %w", err)
	}

	return s.newAuthResult(ctx, user)
}

// Refresh refreshes access token.
func (s *authService) Refresh(ctx context.Context, input request.RefreshTokenInput) (*RefreshResult, error) {
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	tokenHash := s.tokens.HashToken(refreshToken)
	storedToken, err := s.refreshTokens.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("refresh token is invalid")
		}
		return nil, fmt.Errorf("get refresh token failed: %w", err)
	}
	now := time.Now().UTC()
	if storedToken.RevokedAt != nil {
		return nil, fmt.Errorf("refresh token is revoked")
	}
	if !storedToken.ExpiresAt.After(now) {
		return nil, fmt.Errorf("refresh token is expired")
	}

	user, err := s.getActiveUserByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokens.CreateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token failed: %w", err)
	}

	return &RefreshResult{AccessToken: accessToken}, nil
}

// Logout revokes a refresh token.
func (s *authService) Logout(ctx context.Context, input request.LogoutInput) error {
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if refreshToken == "" {
		return fmt.Errorf("refresh token is required")
	}

	if err := s.refreshTokens.Revoke(ctx, s.tokens.HashToken(refreshToken), time.Now().UTC()); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("refresh token is invalid")
		}
		return fmt.Errorf("revoke refresh token failed: %w", err)
	}

	return nil
}

// SetupPassword sets up the current user's first login password.
func (s *authService) SetupPassword(ctx context.Context, input request.SetupPasswordInput) error {
	userID, err := parseObjectID(input.UserID)
	if err != nil {
		return err
	}
	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return err
	}
	if input.Password == "" {
		return fmt.Errorf("password is required")
	}
	if err := validatePasswordStrength(input.Password); err != nil {
		return err
	}

	identity, err := s.identities.GetByProviderAndIdentifier(ctx, domain.AuthProviderPhone, phone)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("phone does not match current user")
		}
		return fmt.Errorf("get auth identity failed: %w", err)
	}
	if identity.UserID != userID {
		return fmt.Errorf("phone does not match current user")
	}
	if _, err := s.getActiveUserByID(ctx, userID); err != nil {
		return err
	}

	if s.passwords == nil {
		return fmt.Errorf("password credential repository is not configured")
	}
	if _, err := s.passwords.GetByUserID(ctx, userID); err == nil {
		return fmt.Errorf("password already setup")
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("get password credential failed: %w", err)
	}

	passwordHash, err := s.hashPassword(input.Password)
	if err != nil {
		return fmt.Errorf("hash password failed: %w", err)
	}

	now := time.Now().UTC()
	credential := &domain.UserPasswordCredential{
		ID:                 bson.NewObjectID(),
		UserID:             userID,
		PasswordHash:       passwordHash,
		PasswordAlgo:       passwordAlgorithmBcrypt,
		FailedAttemptCount: 0,
		LockedUntil:        nil,
		LastUsedAt:         nil,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.passwords.Create(ctx, credential); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("password already setup")
		}
		return fmt.Errorf("create password credential failed: %w", err)
	}

	return nil
}

// ChangePassword changes the current user's login password.
func (s *authService) ChangePassword(ctx context.Context, input request.ChangePasswordInput) error {
	userID, err := parseObjectID(input.UserID)
	if err != nil {
		return err
	}
	if input.OldPassword == "" {
		return fmt.Errorf("old password is required")
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
	if s.passwords == nil {
		return fmt.Errorf("password credential repository is not configured")
	}
	credential, err := s.passwords.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("password credential not found")
		}
		return fmt.Errorf("get password credential failed: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(input.OldPassword)); err != nil {
		return fmt.Errorf("password is invalid")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(input.NewPassword)); err == nil {
		return fmt.Errorf("new password must be different")
	}

	passwordHash, err := s.hashPassword(input.NewPassword)
	if err != nil {
		return fmt.Errorf("hash password failed: %w", err)
	}
	now := time.Now().UTC()
	if err := s.passwords.UpdatePassword(ctx, userID, passwordHash, passwordAlgorithmBcrypt, now); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("password credential not found")
		}
		return fmt.Errorf("update password credential failed: %w", err)
	}
	if err := s.refreshTokens.RevokeByUserID(ctx, userID, now); err != nil {
		return fmt.Errorf("revoke refresh tokens failed: %w", err)
	}

	return nil
}

// GetSecurity returns the current user's account security status.
func (s *authService) GetSecurity(ctx context.Context, userID string) (*domain.AuthSecurity, error) {
	objectID, err := parseObjectID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid input")
	}

	if _, err := s.getActiveUserByID(ctx, objectID); err != nil {
		return nil, err
	}

	security := &domain.AuthSecurity{}
	identity, err := s.identities.GetByUserIDAndProvider(ctx, objectID, domain.AuthProviderPhone)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("get phone identity failed: %w", err)
		}
	} else {
		security.Phone = maskPhone(identity.Identifier)
	}

	if s.passwords == nil {
		return nil, fmt.Errorf("password credential repository is not configured")
	}
	if _, err := s.passwords.GetByUserID(ctx, objectID); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return security, nil
		}
		return nil, fmt.Errorf("get password credential failed: %w", err)
	}
	security.PasswordSetup = true

	return security, nil
}

// Me returns the current user.
func (s *authService) Me(ctx context.Context, userID string) (*domain.User, error) {
	objectID, err := parseObjectID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid input")
	}

	return s.getActiveUserByID(ctx, objectID)
}

// UpdateMyProfile updates the current user profile.
func (s *authService) UpdateMyProfile(ctx context.Context, userID string, input request.UpdateMyProfileInput) (*domain.User, error) {
	return s.PatchMyProfile(ctx, userID, ProfilePatchInput{
		Username: &input.Username, Gender: &input.Gender, Birthday: &input.Birthday,
		File: input.File, FileName: input.FileName,
	})
}

// DeleteMe deletes the current user account logically.
func (s *authService) DeleteMe(ctx context.Context, userID string) error {
	objectID, err := parseObjectID(userID)
	if err != nil {
		return fmt.Errorf("invalid input")
	}

	user, err := s.getActiveUserByID(ctx, objectID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	user.Status = domain.UserStatusDeleted
	user.UpdatedAt = now

	if err := s.users.Update(ctx, user); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("delete user failed: %w", err)
	}
	if err := s.identities.DisableByUserID(ctx, objectID, now); err != nil {
		return fmt.Errorf("disable auth identities failed: %w", err)
	}
	if err := s.refreshTokens.RevokeByUserID(ctx, objectID, now); err != nil {
		return fmt.Errorf("revoke refresh tokens failed: %w", err)
	}

	return nil
}

func (s *authService) getOrCreatePhoneUser(ctx context.Context, phone string) (*domain.User, error) {
	identity, err := s.identities.GetByProviderAndIdentifier(ctx, domain.AuthProviderPhone, phone)
	if err == nil {
		return s.getUserByIdentity(ctx, identity)
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("get auth identity failed: %w", err)
	}

	now := time.Now().UTC()
	user := &domain.User{
		ID: bson.NewObjectID(),
		Profile: domain.UserProfile{
			Username:        newDefaultUsername(phone),
			Gender:          "",
			Birthday:        nil,
			AvatarObjectKey: defaultUserAvatarObjectKey,
		},
		Status:    domain.UserStatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user failed: %w", err)
	}

	identity = &domain.AuthIdentity{
		ID:         bson.NewObjectID(),
		UserID:     user.ID,
		Provider:   domain.AuthProviderPhone,
		Identifier: phone,
		Status:     domain.AuthIdentityStatusActive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.identities.Create(ctx, identity); err != nil {
		// Another concurrent first login may have created the unique phone identity.
		// Re-read it so the request can still log in instead of failing on the race.
		existingIdentity, getErr := s.identities.GetByProviderAndIdentifier(ctx, domain.AuthProviderPhone, phone)
		if getErr == nil {
			return s.getUserByIdentity(ctx, existingIdentity)
		}
		return nil, fmt.Errorf("create auth identity failed: %w", err)
	}

	return user, nil
}

func (s *authService) getUserByIdentity(ctx context.Context, identity *domain.AuthIdentity) (*domain.User, error) {
	user, err := s.getActiveUserByID(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *authService) getActiveUserByID(ctx context.Context, userID bson.ObjectID) (*domain.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("get user failed: %w", err)
	}
	if user.Status == domain.UserStatusDeleted {
		return nil, fmt.Errorf("user not found")
	}
	if user.Status == domain.UserStatusDisabled {
		return nil, fmt.Errorf("user is disabled")
	}
	return user, nil
}

func (s *authService) newAuthResult(ctx context.Context, user *domain.User) (*AuthResult, error) {
	accessToken, err := s.tokens.CreateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("create access token failed: %w", err)
	}

	refreshToken, refreshTokenHash, err := s.tokens.CreateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("create refresh token failed: %w", err)
	}

	now := time.Now().UTC()
	storedToken := &domain.RefreshToken{
		ID:        bson.NewObjectID(),
		UserID:    user.ID,
		TokenHash: refreshTokenHash,
		ExpiresAt: now.Add(s.refreshTokenTTL),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.refreshTokens.Create(ctx, storedToken); err != nil {
		return nil, fmt.Errorf("create refresh token failed: %w", err)
	}

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

func normalizePhone(value string) (string, error) {
	phone := strings.TrimSpace(value)
	if phone == "" {
		return "", fmt.Errorf("phone is required")
	}
	phone, err := normalizePNVSPhone(phone)
	if err != nil {
		return "", fmt.Errorf("phone is invalid")
	}
	return "+86" + phone, nil
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if strings.HasPrefix(phone, "+86") && len(phone) == 14 {
		return phone[3:6] + "****" + phone[10:]
	}
	if len(phone) <= 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

func newDefaultUsername(phone string) string {
	digits := digitsOnly(phone)
	if len(digits) > 4 {
		digits = digits[len(digits)-4:]
	}
	return "picpacker_" + digits
}

func (s *authService) hashPassword(password string) (string, error) {
	cost := s.passwordConfig.BcryptCost
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *authService) recordPasswordFailure(ctx context.Context, credential *domain.UserPasswordCredential, now time.Time) error {
	failedAttemptCount := credential.FailedAttemptCount
	if credential.LockedUntil != nil && !credential.LockedUntil.After(now) {
		failedAttemptCount = 0
	}
	failedAttemptCount++
	var lockedUntil *time.Time
	if failedAttemptCount >= s.passwordMaxFailedAttempts() {
		value := now.Add(s.passwordLockDuration())
		lockedUntil = &value
	}
	if err := s.passwords.RecordFailure(ctx, credential.UserID, failedAttemptCount, lockedUntil, now); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("phone or password is invalid")
		}
		return fmt.Errorf("record password login failure failed: %w", err)
	}
	return nil
}

func (s *authService) passwordMaxFailedAttempts() int {
	if s.passwordConfig.MaxFailedAttempts <= 0 {
		return 5
	}
	return s.passwordConfig.MaxFailedAttempts
}

func (s *authService) passwordLockDuration() time.Duration {
	if s.passwordConfig.LockDurationSeconds <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(s.passwordConfig.LockDurationSeconds) * time.Second
}

func validatePasswordStrength(password string) error {
	runes := []rune(password)
	if len(runes) < 8 {
		return fmt.Errorf("password is too short")
	}
	if len(runes) > 32 {
		return fmt.Errorf("password is too long")
	}
	if strings.TrimSpace(password) != password {
		return fmt.Errorf("password has invalid spaces")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false
	for _, r := range runes {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case isAllowedPasswordSpecial(r):
			hasSpecial = true
		default:
			return fmt.Errorf("password has invalid characters")
		}
	}
	categoryCount := 0
	for _, ok := range []bool{hasUpper, hasLower, hasDigit, hasSpecial} {
		if ok {
			categoryCount++
		}
	}
	if categoryCount < 3 {
		return fmt.Errorf("password is too weak")
	}

	return nil
}

func isAllowedPasswordSpecial(r rune) bool {
	return strings.ContainsRune("_#!@$%^&*()+=-", r)
}

func parseRequiredUserGender(value string) (domain.UserGender, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("gender is required")
	}

	switch domain.UserGender(trimmed) {
	case domain.UserGenderMale, domain.UserGenderFemale, domain.UserGenderPrivate:
		return domain.UserGender(trimmed), nil
	default:
		return "", fmt.Errorf("gender is invalid")
	}
}

func parseOptionalBirthday(value string) (*time.Time, error) {
	birthday := strings.TrimSpace(value)
	if birthday == "" {
		return nil, nil
	}

	parsed, err := time.Parse("2006-01-02", birthday)
	if err != nil {
		return nil, fmt.Errorf("birthday is invalid")
	}
	if parsed.After(time.Now().UTC()) {
		return nil, fmt.Errorf("birthday is invalid")
	}

	return &parsed, nil
}

func digitsOnly(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func (s *authService) uploadUserAvatar(ctx context.Context, userID bson.ObjectID, fileName string, file io.ReadSeeker) (string, string, error) {
	body, contentType, err := readUpload(file)
	if err != nil {
		return "", "", fmt.Errorf("invalid input")
	}
	displayBody, displayContentType, err := buildDisplayImage(body)
	if err != nil {
		return "", "", fmt.Errorf("invalid input")
	}

	objectKey := buildUserAvatarObjectKey(userID, fileName, contentType)
	if err := s.uploader.Upload(ctx, objectKey, contentType, bytes.NewReader(body)); err != nil {
		return "", "", fmt.Errorf("upload user avatar failed: %w", err)
	}
	displayObjectKey := buildUserAvatarDisplayObjectKey(userID)
	if err := s.uploader.Upload(ctx, displayObjectKey, displayContentType, bytes.NewReader(displayBody)); err != nil {
		return "", "", fmt.Errorf("upload user avatar failed: %w", err)
	}

	return objectKey, displayObjectKey, nil
}

func buildUserAvatarObjectKey(userID bson.ObjectID, fileName string, contentType string) string {
	ext := extensionForUpload(fileName, contentType)
	return fmt.Sprintf("users/user_%s/profile/avatar/source%s", userID.Hex(), ext)
}

func buildUserAvatarDisplayObjectKey(userID bson.ObjectID) string {
	return fmt.Sprintf("users/user_%s/profile/avatar/display.jpg", userID.Hex())
}
