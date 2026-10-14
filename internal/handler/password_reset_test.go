package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"pack_mate/internal/service"
	"strings"
	"testing"
)

type resetAuthService struct {
	fakeAuthService
	input service.ResetPasswordInput
	err   error
	calls int
}

// ResetPassword records the trusted context and submitted reset fields.
func (s *resetAuthService) ResetPassword(_ context.Context, input service.ResetPasswordInput) error {
	s.input = input
	s.calls++
	return s.err
}

// TestResetPasswordHandler verifies input validation, identity propagation and error mapping.
func TestResetPasswordHandler(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		auth       bool
		err        error
		status     int
	}{
		{"success", `{"phone":"13800138000","code":"123456","new_password":"NewPass2026!","user_id":"attacker"}`, true, nil, 200},
		{"unauthenticated", `{}`, false, nil, 401},
		{"missing password", `{"phone":"13800138000","code":"123456"}`, true, nil, 400},
		{"invalid json", `{`, true, nil, 400},
		{"provider failure", `{"phone":"13800138000","code":"123456","new_password":"NewPass2026!"}`, true, service.ErrPhoneVerificationUnavailable, 502},
		{"conflict", `{"phone":"13800138000","code":"123456","new_password":"NewPass2026!"}`, true, service.ErrPasswordChanged, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &resetAuthService{err: tc.err}
			h := NewAuthHandler(svc, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			if tc.auth {
				c.Set(currentUserIDKey, "trusted-user")
			}
			c.Request = httptest.NewRequest("POST", "/api/v1/auth/password/reset", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.ResetPassword(c)
			if w.Code != tc.status {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if svc.calls > 0 && svc.input.UserID != "trusted-user" {
				t.Fatal("untrusted identity")
			}
			if tc.status == 200 && (!strings.Contains(w.Body.String(), `"reset":true`) || svc.calls != 1) {
				t.Fatal("reset not dispatched")
			}
		})
	}
}
