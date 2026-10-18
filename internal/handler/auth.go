package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pack_mate/internal/config"
	"pack_mate/internal/domain"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/dto/response"
	"pack_mate/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

const (
	defaultUserAvatarObjectKey         = "users/default/avatar.png"
	legacyDefaultUserAvatarObjectKey   = "users/default/profile/avatar/source.jpg"
	previousDefaultUserAvatarObjectKey = "users/default/profile/avatar.jpg"
)

// AuthHandler handles auth HTTP requests.
type AuthHandler struct {
	profileUpload config.ProfileUploadConfig
	svc           service.AuthService
	urlSigner     service.ObjectURLSigner
}

// NewAuthHandler creates an auth handler.
func NewAuthHandler(svc service.AuthService, urlSigner service.ObjectURLSigner, limits ...config.ProfileUploadConfig) *AuthHandler {
	cfg := config.ProfileUploadConfig{}.WithDefaults()
	if len(limits) > 0 {
		cfg = limits[0].WithDefaults()
	}
	return &AuthHandler{svc: svc, urlSigner: urlSigner, profileUpload: cfg}
}

// SendPhoneCode handles phone code sending requests.
func (h *AuthHandler) SendPhoneCode(c *gin.Context) {
	var input request.SendPhoneCodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondPhoneInputError(c, err)
		return
	}

	if err := h.svc.SendPhoneCode(c.Request.Context(), input); err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.SendPhoneCodeResponse{Sent: true})
}

// LoginWithPhone handles phone login requests.
func (h *AuthHandler) LoginWithPhone(c *gin.Context) {
	var input request.PhoneLoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondPhoneInputError(c, err)
		return
	}

	result, err := h.svc.LoginWithPhone(c.Request.Context(), input)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	authResponse, err := h.buildAuthResponse(c.Request.Context(), result)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, authResponse)
}

func respondPhoneInputError(c *gin.Context, err error) {
	message := "invalid input"
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) && len(validationErrors) > 0 {
		fieldError := validationErrors[0]
		switch fieldError.Field() {
		case "Phone":
			message = "phone is required"
		case "Code":
			if fieldError.Tag() == "required" {
				message = "code is required"
			} else {
				message = "phone code is invalid"
			}
		}
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": message})
}

// LoginWithPhonePassword handles phone password login requests.
func (h *AuthHandler) LoginWithPhonePassword(c *gin.Context) {
	var input request.PhonePasswordLoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if strings.TrimSpace(input.Phone) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}
	if input.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}

	result, err := h.svc.LoginWithPhonePassword(c.Request.Context(), input)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	authResponse, err := h.buildAuthResponse(c.Request.Context(), result)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, authResponse)
}

// Refresh handles refresh token requests.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var input request.RefreshTokenInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if strings.TrimSpace(input.RefreshToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	result, err := h.svc.Refresh(c.Request.Context(), input)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.RefreshAccessTokenResponse{
		AccessToken: result.AccessToken,
	})
}

// Logout handles logout requests.
func (h *AuthHandler) Logout(c *gin.Context) {
	var input request.LogoutInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if strings.TrimSpace(input.RefreshToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	if err := h.svc.Logout(c.Request.Context(), input); err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.LogoutResponse{LoggedOut: true})
}

// SetupPassword handles first-time password setup requests.
func (h *AuthHandler) SetupPassword(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var input request.SetupPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if strings.TrimSpace(input.Phone) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}
	if input.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}
	input.UserID = userID

	if err := h.svc.SetupPassword(c.Request.Context(), input); err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.SetupPasswordResponse{Setup: true})
}

// ChangePassword handles login password change requests.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var input request.ChangePasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if input.OldPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old password is required"})
		return
	}
	if input.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password is required"})
		return
	}
	input.UserID = userID

	if err := h.svc.ChangePassword(c.Request.Context(), input); err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.ChangePasswordResponse{Changed: true})
}

// GetSecurity handles account security status requests.
func (h *AuthHandler) GetSecurity(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	security, err := h.svc.GetSecurity(c.Request.Context(), userID)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.AuthSecurityResponse{
		Phone:         security.Phone,
		PasswordSetup: security.PasswordSetup,
	})
}

// Me handles current user requests.
func (h *AuthHandler) Me(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	userResponse, err := h.buildUserResponse(c.Request.Context(), user)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, userResponse)
}

// UpdateMyProfile handles current user profile update requests.
func (h *AuthHandler) UpdateMyProfile(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	patch, cleanup, err := h.parseProfileForm(c)
	defer cleanup()
	if err != nil {
		respondProfileInputError(c, err)
		return
	}
	input := request.UpdateMyProfileInput{File: patch.File, FileName: patch.FileName}
	if patch.Username != nil {
		input.Username = *patch.Username
	}
	if patch.Gender != nil {
		input.Gender = *patch.Gender
	}
	if patch.Birthday != nil {
		input.Birthday = *patch.Birthday
	}
	if strings.TrimSpace(input.Username) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}
	if strings.TrimSpace(input.Gender) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gender is required"})
		return
	}

	user, err := h.svc.UpdateMyProfile(c.Request.Context(), userID, input)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	userResponse, err := h.buildUserResponse(c.Request.Context(), user)
	if err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, userResponse)
}

// DeleteMe handles current account deletion requests.
func (h *AuthHandler) DeleteMe(c *gin.Context) {
	userID, ok := CurrentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.svc.DeleteMe(c.Request.Context(), userID); err != nil {
		respondAuthError(c, err)
		return
	}

	c.JSON(http.StatusOK, response.DeleteMeResponse{Deleted: true})
}

func respondAuthError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := err.Error()
	switch {
	case errors.Is(err, service.ErrAvatarTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, service.ErrEmptyProfilePatch):
		status = http.StatusBadRequest
	case errors.Is(err, service.ErrPasswordChanged):
		status = http.StatusConflict
	case errors.Is(err, service.ErrPhoneCodeRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(err, service.ErrPhoneVerificationUnavailable):
		status = http.StatusBadGateway
	case strings.Contains(message, "invalid input"),
		strings.Contains(message, "phone is required"),
		strings.Contains(message, "phone is invalid"),
		strings.Contains(message, "phone code is required"),
		strings.Contains(message, "phone code is invalid"),
		strings.Contains(message, "code is required"),
		strings.Contains(message, "refresh token is required"),
		strings.Contains(message, "password is required"),
		strings.Contains(message, "old password is required"),
		strings.Contains(message, "new password is required"),
		strings.Contains(message, "password is too short"),
		strings.Contains(message, "password is too long"),
		strings.Contains(message, "password is too weak"),
		strings.Contains(message, "password has invalid spaces"),
		strings.Contains(message, "password has invalid characters"),
		strings.Contains(message, "new password must be different"),
		strings.Contains(message, "username is required"),
		strings.Contains(message, "gender is required"),
		strings.Contains(message, "avatar is required"),
		strings.Contains(message, "username is too long"),
		strings.Contains(message, "gender is invalid"),
		strings.Contains(message, "birthday is invalid"):
		status = http.StatusBadRequest
	case strings.Contains(message, "access token is invalid"),
		strings.Contains(message, "access token is expired"),
		strings.Contains(message, "refresh token is invalid"),
		strings.Contains(message, "refresh token is expired"),
		strings.Contains(message, "refresh token is revoked"),
		strings.Contains(message, "phone or password is invalid"),
		strings.Contains(message, "password is invalid"):
		status = http.StatusUnauthorized
	case strings.Contains(message, "user is disabled"):
		status = http.StatusForbidden
	case strings.Contains(message, "password is locked"):
		status = http.StatusForbidden
	case strings.Contains(message, "phone does not match current user"):
		status = http.StatusForbidden
	case strings.Contains(message, "user not found"):
		status = http.StatusNotFound
	case strings.Contains(message, "password credential not found"):
		status = http.StatusNotFound
	case strings.Contains(message, "create auth identity failed"),
		strings.Contains(message, "password already setup"):
		status = http.StatusConflict
	case strings.Contains(message, "upload user avatar failed"):
		status = http.StatusBadGateway
	case strings.Contains(message, "sign user avatar url failed"):
		status = http.StatusInternalServerError
	}
	c.JSON(status, gin.H{"error": message})
}

func (h *AuthHandler) buildAuthResponse(ctx context.Context, result *service.AuthResult) (response.AuthResponse, error) {
	userResponse, err := h.buildUserResponse(ctx, result.User)
	if err != nil {
		return response.AuthResponse{}, err
	}

	return response.AuthResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		User:         userResponse,
	}, nil
}

func (h *AuthHandler) buildUserResponse(ctx context.Context, user *domain.User) (response.UserResponse, error) {
	avatarSourceURL, err := h.signAvatarURL(ctx, user.Profile.AvatarObjectKey)
	if err != nil {
		return response.UserResponse{}, err
	}
	avatarObjectKey := strings.TrimSpace(user.Profile.AvatarDisplayObjectKey)
	if avatarObjectKey == "" {
		avatarObjectKey = user.Profile.AvatarObjectKey
	}
	avatarURL, err := h.signAvatarURL(ctx, avatarObjectKey)
	if err != nil {
		return response.UserResponse{}, err
	}

	return response.UserResponse{
		ID: user.ID.Hex(),
		Profile: response.UserProfileResponse{
			Username:        user.Profile.Username,
			Gender:          string(user.Profile.Gender),
			Birthday:        formatUserBirthday(user.Profile.Birthday),
			AvatarURL:       avatarURL,
			AvatarSourceURL: avatarSourceURL,
		},
		Status: string(user.Status),
	}, nil
}

func (h *AuthHandler) signAvatarURL(ctx context.Context, objectKey string) (string, error) {
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return "", nil
	}
	objectKey = normalizeAvatarObjectKey(objectKey)
	if h.urlSigner == nil {
		return "", fmt.Errorf("sign user avatar url failed: url signer is not configured")
	}

	signedURL, err := h.urlSigner.SignGetURL(ctx, objectKey)
	if err != nil {
		return "", fmt.Errorf("sign user avatar url failed: %w", err)
	}

	return signedURL, nil
}

func normalizeAvatarObjectKey(objectKey string) string {
	if objectKey == legacyDefaultUserAvatarObjectKey || objectKey == previousDefaultUserAvatarObjectKey {
		return defaultUserAvatarObjectKey
	}
	return objectKey
}

func formatUserBirthday(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02")
}
