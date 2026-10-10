package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"pack_mate/internal/domain"
)

type fakeLaunchScreenHandlerService struct {
	config *domain.LaunchScreen
	err    error
}

// Get returns the configured launch screen for handler tests.
func (s *fakeLaunchScreenHandlerService) Get(context.Context) (*domain.LaunchScreen, error) {
	return s.config, s.err
}

// TestLaunchScreenHandlerReturnsPublicResponse verifies public response fields and cache headers.
func TestLaunchScreenHandlerReturnsPublicResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	handler := NewLaunchScreenHandler(&fakeLaunchScreenHandlerService{config: &domain.LaunchScreen{
		Enabled: true, Revision: "2026-10-08-02", Type: domain.LaunchScreenTypeVideo,
		MediaKey: "https://signed.example/video.mp4", PosterKey: "https://signed.example/poster.webp",
		DurationMS: 5000, SkipEnabled: true,
	}})
	context.Request = httptest.NewRequest("GET", "/api/v1/app/launch-screen", nil)
	handler.Get(context)
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("missing no-cache response header")
	}
	if recorder.Body.String() == "" || !containsAll(recorder.Body.String(), `"revision":"2026-10-08-02"`, `"type":"video"`, `"poster_url":"https://signed.example/poster.webp"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

// TestLaunchScreenHandlerReturnsInternalError verifies downstream failures are not reported as disabled.
func TestLaunchScreenHandlerReturnsInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	handler := NewLaunchScreenHandler(&fakeLaunchScreenHandlerService{err: errors.New("database unavailable")})
	context.Request = httptest.NewRequest("GET", "/api/v1/app/launch-screen", nil)
	handler.Get(context)
	if recorder.Code != 500 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
