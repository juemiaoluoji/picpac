package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pack_mate/internal/domain"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/service"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type fakeAuthService struct {
	err                 error
	sendPhoneCodeCalls  int
	phoneLoginCalls     int
	passwordLoginErr    error
	setupPasswordErr    error
	changePasswordErr   error
	user                *domain.User
	security            *domain.AuthSecurity
	deletedUserID       string
	passwordLoginInput  request.PhonePasswordLoginInput
	setupPasswordInput  request.SetupPasswordInput
	changePasswordInput request.ChangePasswordInput
}

type fakeObjectURLSigner struct {
	err error
}

func (s fakeObjectURLSigner) SignGetURL(_ context.Context, objectKey string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return "https://signed.example/" + strings.TrimLeft(objectKey, "/") + "?Expires=3600&Signature=test", nil
}

func (s *fakeAuthService) SendPhoneCode(_ context.Context, _ request.SendPhoneCodeInput) error {
	s.sendPhoneCodeCalls++
	return s.err
}

func (s *fakeAuthService) LoginWithPhone(_ context.Context, _ request.PhoneLoginInput) (*service.AuthResult, error) {
	s.phoneLoginCalls++
	if s.err != nil {
		return nil, s.err
	}
	return &service.AuthResult{AccessToken: "access", RefreshToken: "refresh", User: s.defaultUser()}, nil
}

func (s *fakeAuthService) LoginWithPhonePassword(_ context.Context, input request.PhonePasswordLoginInput) (*service.AuthResult, error) {
	s.passwordLoginInput = input
	if s.passwordLoginErr != nil {
		return nil, s.passwordLoginErr
	}
	return &service.AuthResult{AccessToken: "access", RefreshToken: "refresh", User: s.defaultUser()}, nil
}

func (s *fakeAuthService) Refresh(_ context.Context, _ request.RefreshTokenInput) (*service.RefreshResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.RefreshResult{AccessToken: "access"}, nil
}

func (s *fakeAuthService) Logout(_ context.Context, _ request.LogoutInput) error {
	return s.err
}

func (s *fakeAuthService) SetupPassword(_ context.Context, input request.SetupPasswordInput) error {
	s.setupPasswordInput = input
	if s.setupPasswordErr != nil {
		return s.setupPasswordErr
	}
	return nil
}

func (s *fakeAuthService) ChangePassword(_ context.Context, input request.ChangePasswordInput) error {
	s.changePasswordInput = input
	if s.changePasswordErr != nil {
		return s.changePasswordErr
	}
	return nil
}

func (s *fakeAuthService) GetSecurity(_ context.Context, _ string) (*domain.AuthSecurity, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.security != nil {
		return s.security, nil
	}
	return &domain.AuthSecurity{Phone: "138****8000", PasswordSetup: true}, nil
}

func (s *fakeAuthService) Me(_ context.Context, _ string) (*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.defaultUser(), nil
}

func (s *fakeAuthService) UpdateMyProfile(_ context.Context, _ string, input request.UpdateMyProfileInput) (*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}

	user := s.defaultUser()
	user.Profile.Username = strings.TrimSpace(input.Username)
	user.Profile.Gender = domain.UserGender(strings.TrimSpace(input.Gender))
	if strings.TrimSpace(input.Birthday) != "" {
		birthday, err := time.Parse("2006-01-02", input.Birthday)
		if err != nil {
			return nil, err
		}
		user.Profile.Birthday = &birthday
	}
	user.Profile.AvatarObjectKey = "users/user_1/profile/avatar/source.png"

	return user, nil
}

func (s *fakeAuthService) DeleteMe(_ context.Context, userID string) error {
	s.deletedUserID = userID
	return s.err
}

func (s *fakeAuthService) defaultUser() *domain.User {
	if s.user != nil {
		return s.user
	}
	return &domain.User{
		ID: bson.NewObjectID(),
		Profile: domain.UserProfile{
			Username:        "用户8000",
			Gender:          "",
			AvatarObjectKey: "users/default/avatar.png",
		},
		Status: domain.UserStatusCreated,
	}
}

func TestSendPhoneCodeHandlerReturnsSent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{}, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/code", authHandler.SendPhoneCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/code", bytes.NewBufferString(`{"phone":"13800138000"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"sent":true`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestSendPhoneCodeHandlerRequiresPhone(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authService := &fakeAuthService{}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/code", authHandler.SendPhoneCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/code", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest || recorder.Body.String() != `{"error":"phone is required"}` {
		t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
	}
	if authService.sendPhoneCodeCalls != 0 {
		t.Fatal("invalid input reached auth service")
	}
}

func TestLoginWithPhoneHandlerReturnsTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{}, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/login", authHandler.LoginWithPhone)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/login", bytes.NewBufferString(`{"phone":"13800138000","code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			Profile struct {
				Username        string `json:"username"`
				AvatarURL       string `json:"avatar_url"`
				AvatarSourceURL string `json:"avatar_source_url"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.User.Profile.Username != "用户8000" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.User.Profile.AvatarURL != "https://signed.example/users/default/avatar.png?Expires=3600&Signature=test" {
		t.Fatalf("unexpected avatar_url: %s", resp.User.Profile.AvatarURL)
	}
	if resp.User.Profile.AvatarSourceURL != "https://signed.example/users/default/avatar.png?Expires=3600&Signature=test" {
		t.Fatalf("unexpected avatar_source_url: %s", resp.User.Profile.AvatarSourceURL)
	}
}

func TestLoginWithPhoneCodeHandlerReturnsTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{}, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/code/login", authHandler.LoginWithPhone)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/code/login", bytes.NewBufferString(`{"phone":"13800138000","code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"access_token":"access"`) || !strings.Contains(recorder.Body.String(), `"refresh_token":"refresh"`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestLoginWithPhonePasswordHandlerReturnsTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authService := &fakeAuthService{}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/password/login", authHandler.LoginWithPhonePassword)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/password/login", bytes.NewBufferString(`{"phone":"13800138000","password":"Trip2026Pass"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if authService.passwordLoginInput.Phone != "13800138000" || authService.passwordLoginInput.Password != "Trip2026Pass" {
		t.Fatalf("unexpected password login input: %+v", authService.passwordLoginInput)
	}
	if !strings.Contains(recorder.Body.String(), `"access_token":"access"`) || !strings.Contains(recorder.Body.String(), `"refresh_token":"refresh"`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestLoginWithPhonePasswordHandlerMapsLockedPassword(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{passwordLoginErr: errors.New("password is locked")}, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/password/login", authHandler.LoginWithPhonePassword)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/password/login", bytes.NewBufferString(`{"phone":"13800138000","password":"Trip2026Pass"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRefreshHandlerReturnsAccessTokenOnly(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{}, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/refresh", authHandler.Refresh)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"refresh"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["access_token"] == "" {
		t.Fatalf("expected access token, got %+v", resp)
	}
	if _, ok := resp["refresh_token"]; ok {
		t.Fatalf("did not expect refresh_token in response: %+v", resp)
	}
}

func TestSetupPasswordHandlerUsesCurrentUserID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authService := &fakeAuthService{user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.POST("/api/v1/auth/password/setup", authMiddleware.RequireAuth(), authHandler.SetupPassword)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/setup", bytes.NewBufferString(`{"phone":"13800138000","password":"Trip2026Pass"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if authService.setupPasswordInput.UserID != userID.Hex() || authService.setupPasswordInput.Phone != "13800138000" || authService.setupPasswordInput.Password != "Trip2026Pass" {
		t.Fatalf("unexpected setup password input: %+v", authService.setupPasswordInput)
	}
	if !strings.Contains(recorder.Body.String(), `"setup":true`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestSetupPasswordHandlerMapsWeakPassword(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{setupPasswordErr: errors.New("password is too weak"), user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.POST("/api/v1/auth/password/setup", authMiddleware.RequireAuth(), authHandler.SetupPassword)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/setup", bytes.NewBufferString(`{"phone":"13800138000","password":"12345678"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestChangePasswordHandlerUsesCurrentUserID(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authService := &fakeAuthService{user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.PUT("/api/v1/auth/password", authMiddleware.RequireAuth(), authHandler.ChangePassword)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(`{"old_password":"Trip2026Pass","new_password":"NewTrip2027"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if authService.changePasswordInput.UserID != userID.Hex() ||
		authService.changePasswordInput.OldPassword != "Trip2026Pass" ||
		authService.changePasswordInput.NewPassword != "NewTrip2027" {
		t.Fatalf("unexpected change password input: %+v", authService.changePasswordInput)
	}
	if !strings.Contains(recorder.Body.String(), `"changed":true`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestChangePasswordHandlerMapsInvalidPassword(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{changePasswordErr: errors.New("password is invalid"), user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.PUT("/api/v1/auth/password", authMiddleware.RequireAuth(), authHandler.ChangePassword)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(`{"old_password":"Wrong2026Pass","new_password":"NewTrip2027"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestLoginWithPhoneHandlerRejectsMissingCode(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authService := &fakeAuthService{}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/login", authHandler.LoginWithPhone)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/login", bytes.NewBufferString(`{"phone":"13800138000"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if recorder.Body.String() != `{"error":"code is required"}` {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
	if authService.phoneLoginCalls != 0 {
		t.Fatal("invalid input reached auth service")
	}
}

func TestLoginWithPhoneHandlerValidatesCodeShape(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"12345", "1234567", "abcdef"} {
		t.Run(code, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			router := gin.New()
			authService := &fakeAuthService{}
			authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
			router.POST("/api/v1/auth/phone/code/login", authHandler.LoginWithPhone)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/code/login", bytes.NewBufferString(`{"phone":"13800138000","code":"`+code+`"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusBadRequest || recorder.Body.String() != `{"error":"phone code is invalid"}` {
				t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
			}
			if authService.phoneLoginCalls != 0 {
				t.Fatal("invalid input reached auth service")
			}
		})
	}
}

func TestAuthHandlerMapsProviderRateLimit(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{err: service.ErrPhoneCodeRateLimited}, fakeObjectURLSigner{})
	router.POST("/api/v1/auth/phone/code", authHandler.SendPhoneCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/phone/code", bytes.NewBufferString(`{"phone":"13800138000"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", recorder.Code)
	}
	if recorder.Body.String() != `{"error":"phone code send too frequently"}` {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestAuthHandlerMapsPhoneVerificationUnavailable(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"/api/v1/auth/phone/code", "/api/v1/auth/phone/code/login"} {
		t.Run(endpoint, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			router := gin.New()
			authHandler := NewAuthHandler(&fakeAuthService{err: service.ErrPhoneVerificationUnavailable}, fakeObjectURLSigner{})
			if endpoint == "/api/v1/auth/phone/code" {
				router.POST(endpoint, authHandler.SendPhoneCode)
			} else {
				router.POST(endpoint, authHandler.LoginWithPhone)
			}

			body := `{"phone":"13800138000"}`
			if strings.HasSuffix(endpoint, "/login") {
				body = `{"phone":"13800138000","code":"123456"}`
			}
			req := httptest.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("expected 502, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if recorder.Body.String() != `{"error":"phone verification service is unavailable"}` {
				t.Fatalf("unexpected body: %s", recorder.Body.String())
			}
		})
	}
}

func TestMeHandlerReturnsCurrentUser(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.GET("/api/v1/me", authMiddleware.RequireAuth(), authHandler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), userID.Hex()) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestGetSecurityHandlerReturnsCurrentSecurity(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authService := &fakeAuthService{
		user:     &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated},
		security: &domain.AuthSecurity{Phone: "138****8000", PasswordSetup: false},
	}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.GET("/api/v1/auth/security", authMiddleware.RequireAuth(), authHandler.GetSecurity)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/security", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var resp struct {
		Phone         string `json:"phone"`
		PasswordSetup bool   `json:"password_setup"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Phone != "138****8000" || resp.PasswordSetup {
		t.Fatalf("unexpected security response: %+v", resp)
	}
}

func TestMeHandlerReturnsDisplayAndSourceAvatarURLs(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{user: &domain.User{
		ID: userID,
		Profile: domain.UserProfile{
			Username:               "用户8000",
			AvatarObjectKey:        "users/user_1/profile/avatar/source.png",
			AvatarDisplayObjectKey: "users/user_1/profile/avatar/display.jpg",
		},
		Status: domain.UserStatusCreated,
	}}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.GET("/api/v1/me", authMiddleware.RequireAuth(), authHandler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var resp struct {
		Profile struct {
			AvatarURL       string `json:"avatar_url"`
			AvatarSourceURL string `json:"avatar_source_url"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Profile.AvatarURL != "https://signed.example/users/user_1/profile/avatar/display.jpg?Expires=3600&Signature=test" {
		t.Fatalf("unexpected avatar_url: %s", resp.Profile.AvatarURL)
	}
	if resp.Profile.AvatarSourceURL != "https://signed.example/users/user_1/profile/avatar/source.png?Expires=3600&Signature=test" {
		t.Fatalf("unexpected avatar_source_url: %s", resp.Profile.AvatarSourceURL)
	}
}

func TestMeHandlerNormalizesLegacyDefaultAvatarURL(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{user: &domain.User{
		ID: userID,
		Profile: domain.UserProfile{
			Username:        "用户8000",
			AvatarObjectKey: "users/default/profile/avatar/source.jpg",
		},
		Status: domain.UserStatusCreated,
	}}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.GET("/api/v1/me", authMiddleware.RequireAuth(), authHandler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var resp struct {
		Profile struct {
			AvatarURL       string `json:"avatar_url"`
			AvatarSourceURL string `json:"avatar_source_url"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	expectedAvatarURL := "https://signed.example/users/default/avatar.png?Expires=3600&Signature=test"
	if resp.Profile.AvatarURL != expectedAvatarURL || resp.Profile.AvatarSourceURL != expectedAvatarURL {
		t.Fatalf("unexpected avatar response: %+v", resp.Profile)
	}
}

func TestDeleteMeHandlerReturnsDeleted(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authService := &fakeAuthService{user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "用户8000", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}
	authHandler := NewAuthHandler(authService, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.DELETE("/api/v1/auth/me", authMiddleware.RequireAuth(), authHandler.DeleteMe)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if authService.deletedUserID != userID.Hex() {
		t.Fatalf("expected user %s to be deleted, got %s", userID.Hex(), authService.deletedUserID)
	}
	if !strings.Contains(recorder.Body.String(), `"deleted":true`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestUpdateMyProfileHandlerReturnsUpdatedUser(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{user: &domain.User{ID: userID, Profile: domain.UserProfile{Username: "旧用户名", AvatarObjectKey: "users/default/avatar.png"}, Status: domain.UserStatusCreated}}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.PUT("/api/v1/me/profile", authMiddleware.RequireAuth(), authHandler.UpdateMyProfile)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("username", "新用户"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("gender", "female"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	if err := writer.WriteField("birthday", "1998-08-20"); err != nil {
		t.Fatalf("WriteField returned error: %v", err)
	}
	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader([]byte("fake image"))); err != nil {
		t.Fatalf("Copy returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/me/profile", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"username":"新用户"`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	authHandler := NewAuthHandler(&fakeAuthService{}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(service.NewTokenService("test-secret", time.Hour), authHandler.svc)
	router.GET("/api/v1/me", authMiddleware.RequireAuth(), authHandler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestAuthMiddlewareRejectsDeletedUser(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	router := gin.New()
	tokenService := service.NewTokenService("test-secret", time.Hour)
	userID := bson.NewObjectID()
	token, err := tokenService.CreateAccessToken(userID)
	if err != nil {
		t.Fatalf("CreateAccessToken returned error: %v", err)
	}
	authHandler := NewAuthHandler(&fakeAuthService{err: errors.New("user not found")}, fakeObjectURLSigner{})
	authMiddleware := NewAuthMiddleware(tokenService, authHandler.svc)
	router.GET("/api/v1/me", authMiddleware.RequireAuth(), authHandler.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

// ResetPassword implements password reset for HTTP tests.
func (s *fakeAuthService) ResetPassword(_ context.Context, _ service.ResetPasswordInput) error {
	return nil
}

// PatchMyProfile implements profile patching for HTTP tests.
func (s *fakeAuthService) PatchMyProfile(_ context.Context, _ string, _ service.ProfilePatchInput) (*domain.User, error) {
	return &domain.User{}, nil
}
