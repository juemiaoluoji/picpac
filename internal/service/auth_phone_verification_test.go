package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"pack_mate/internal/dto/request"
)

func TestDevPhoneLoginUsesConfiguredCode(t *testing.T) {
	t.Parallel()
	cfg := validAuthConfig()
	cfg.PhoneCode.DevFixedCode = "654321"
	svc := NewAuthService(
		&fakeUserRepository{},
		&fakeAuthIdentityRepository{},
		&fakeRefreshTokenRepository{},
		&fakeUserPasswordCredentialRepository{},
		&fakeAuthUploadService{},
		&recordingPhoneVerificationService{},
		NewTokenService(cfg.AccessTokenSecret, time.Duration(cfg.AccessTokenTTLSeconds)*time.Second),
		cfg,
		"dev",
	)
	if _, err := svc.LoginWithPhone(context.Background(), request.PhoneLoginInput{Phone: "13800138000", Code: "123456"}); err == nil {
		t.Fatal("dev accepted a code other than auth.phone_code.dev_fixed_code")
	}
	if _, err := svc.LoginWithPhone(context.Background(), request.PhoneLoginInput{Phone: "13800138000", Code: "654321"}); err != nil {
		t.Fatalf("dev rejected auth.phone_code.dev_fixed_code: %v", err)
	}
}

func TestDevPhoneCodeLoginFlowFailsThenSucceedsWithMockedDependencies(t *testing.T) {
	t.Parallel()
	users := &fakeUserRepository{}
	identities := &fakeAuthIdentityRepository{}
	refreshTokens := &fakeRefreshTokenRepository{}
	passwords := &fakeUserPasswordCredentialRepository{}
	uploader := &fakeAuthUploadService{}
	provider := &recordingPhoneVerificationService{err: errors.New("provider must not be called"), verifyErr: errors.New("provider must not be called")}
	cfg := validAuthConfig()
	svc := NewAuthService(
		users,
		identities,
		refreshTokens,
		passwords,
		uploader,
		provider,
		NewTokenService(cfg.AccessTokenSecret, time.Duration(cfg.AccessTokenTTLSeconds)*time.Second),
		cfg,
		"dev",
	)
	ctx := context.Background()
	phone := "13800138000"
	if err := svc.SendPhoneCode(ctx, request.SendPhoneCodeInput{Phone: phone}); err != nil {
		t.Fatalf("SendPhoneCode returned error: %v", err)
	}
	if _, err := svc.LoginWithPhone(ctx, request.PhoneLoginInput{Phone: phone, Code: "654321"}); err == nil {
		t.Fatal("expected the first login with an invalid dev code to fail")
	}
	if users.created != nil || identities.created != nil || refreshTokens.created != nil {
		t.Fatal("failed verification must not create login state")
	}
	result, err := svc.LoginWithPhone(ctx, request.PhoneLoginInput{Phone: phone, Code: "123456"})
	if err != nil || result == nil || result.User == nil || result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatalf("unexpected successful login result: result=%+v error=%v", result, err)
	}
	if users.created == nil || identities.created == nil || refreshTokens.created == nil {
		t.Fatal("successful login did not persist the mocked user, identity, and refresh token")
	}
	if provider.phone != "" || provider.verifyCalls != 0 || passwords.created != nil || len(uploader.objectKeys) != 0 {
		t.Fatal("dev phone login accessed an external dependency")
	}
}

func TestProdPhoneLoginUsesAliyunVerification(t *testing.T) {
	t.Parallel()
	provider := newTestPNVSService(t)
	var actions []string
	provider.client.Transport = pnvsTestTransport(func(req *http.Request) (*http.Response, error) {
		action := ""
		for key, values := range req.Header {
			if strings.EqualFold(key, "x-acs-action") {
				action = values[0]
			}
		}
		actions = append(actions, action)
		body := `{"Code":"OK","Success":true}`
		switch action {
		case "SendSmsVerifyCode":
			if req.URL.Query().Get("TemplateParam") != `{"code":"##code##","min":"5"}` {
				t.Fatal("prod must request a provider-generated code")
			}
		case "CheckSmsVerifyCode":
			body = `{"Code":"OK","Success":true,"Model":{"VerifyResult":"UNKNOWN"}}`
			if req.URL.Query().Get("VerifyCode") == "654321" {
				body = `{"Code":"OK","Success":true,"Model":{"VerifyResult":"PASS"}}`
			}
		default:
			t.Fatalf("unexpected action %q", action)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	s := newTestAuthService(&fakeUserRepository{}, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, nil).(*authService)
	s.verification = provider
	ctx := context.Background()
	if err := s.SendPhoneCode(ctx, request.SendPhoneCodeInput{Phone: "13800138000"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoginWithPhone(ctx, request.PhoneLoginInput{Phone: "13800138000", Code: "123456"}); err == nil {
		t.Fatal("prod accepted a code rejected by the provider")
	}
	result, err := s.LoginWithPhone(ctx, request.PhoneLoginInput{Phone: "13800138000", Code: "654321"})
	if err != nil || result == nil || result.AccessToken == "" {
		t.Fatalf("prod login failed: %v", err)
	}
	if strings.Join(actions, ",") != "SendSmsVerifyCode,CheckSmsVerifyCode,CheckSmsVerifyCode" {
		t.Fatalf("unexpected provider flow: %v", actions)
	}
}

func TestProdPhoneVerificationFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		code        string
		providerErr error
		wantCalls   int
	}{
		{name: "invalid code shape", code: "abcdef"},
		{name: "wrong code", code: "654321", wantCalls: 1},
		{name: "provider error", code: "123456", providerErr: errors.New("provider unavailable"), wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &recordingPhoneVerificationService{verifyErr: tc.providerErr}
			users := &fakeUserRepository{}
			s := newTestAuthService(users, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, provider)
			if _, err := s.LoginWithPhone(context.Background(), request.PhoneLoginInput{Phone: "13800138000", Code: tc.code}); err == nil {
				t.Fatal("expected login failure")
			}
			if provider.verifyCalls != tc.wantCalls || users.created != nil {
				t.Fatal("failed verification reached account creation")
			}
		})
	}
}

func TestProdPhoneSendFailure(t *testing.T) {
	t.Parallel()
	providerErr := errors.New("provider unavailable")
	provider := &recordingPhoneVerificationService{err: providerErr}
	s := newTestAuthService(&fakeUserRepository{}, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, provider)
	err := s.SendPhoneCode(context.Background(), request.SendPhoneCodeInput{Phone: "13800138000"})
	if !errors.Is(err, ErrPhoneVerificationUnavailable) || !errors.Is(err, providerErr) {
		t.Fatalf("expected public and provider errors, got %v", err)
	}
	if err.Error() != "phone verification service is unavailable" {
		t.Fatalf("unexpected public error: %v", err)
	}
}

func TestProdPhoneVerificationFailureIsClientSafe(t *testing.T) {
	t.Parallel()
	providerErr := errors.New("provider diagnostic must stay private")
	provider := &recordingPhoneVerificationService{verifyErr: providerErr}
	s := newTestAuthService(&fakeUserRepository{}, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, provider)

	_, err := s.LoginWithPhone(context.Background(), request.PhoneLoginInput{Phone: "13800138000", Code: "123456"})
	if !errors.Is(err, ErrPhoneVerificationUnavailable) || !errors.Is(err, providerErr) {
		t.Fatalf("expected public and provider errors, got %v", err)
	}
	if err.Error() != "phone verification service is unavailable" {
		t.Fatalf("unexpected public error: %v", err)
	}
}

func TestPhoneAuthRejectsUnsupportedNumbers(t *testing.T) {
	t.Parallel()
	for _, env := range []string{"dev", "prod"} {
		s := &authService{env: env}
		for _, phone := range []string{"", "+12025550123", "１３８００１３８０００", "invalid"} {
			if err := s.SendPhoneCode(context.Background(), request.SendPhoneCodeInput{Phone: phone}); err == nil {
				t.Fatalf("%s accepted phone %q", env, phone)
			}
			if _, err := s.LoginWithPhone(context.Background(), request.PhoneLoginInput{Phone: phone, Code: "123456"}); err == nil {
				t.Fatalf("%s accepted login phone %q", env, phone)
			}
		}
	}
}
