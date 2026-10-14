package handler

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"pack_mate/internal/config"
	"pack_mate/internal/domain"
	"pack_mate/internal/service"
)

type profilePatchService struct {
	fakeAuthService
	input  service.ProfilePatchInput
	userID string
	calls  int
	err    error
}

// PatchMyProfile records the parsed patch and trusted identity.
func (s *profilePatchService) PatchMyProfile(_ context.Context, id string, input service.ProfilePatchInput) (*domain.User, error) {
	s.calls++
	s.input = input
	s.userID = id
	if s.err != nil {
		return nil, s.err
	}
	return &domain.User{Profile: domain.UserProfile{Username: "stored", AvatarObjectKey: defaultUserAvatarObjectKey}}, nil
}

// TestProfilePatchHandler verifies multipart presence, validation and response mapping.
func TestProfilePatchHandler(t *testing.T) {
	for _, name := range []string{"name", "clear birthday", "avatar", "duplicate", "unknown", "wrong file", "oversize", "oversize body", "malformed", "unauthenticated", "provider failure", "database failure", "empty"} {
		t.Run(name, func(t *testing.T) {
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			switch name {
			case "clear birthday":
				_ = writer.WriteField("birthday", "")
			case "duplicate":
				_ = writer.WriteField("username", "one")
				_ = writer.WriteField("username", "two")
			case "unknown":
				_ = writer.WriteField("user_id", "attacker")
			case "avatar", "oversize", "wrong file":
				field := "avatar"
				if name == "wrong file" {
					field = "other"
				}
				part, _ := writer.CreateFormFile(field, "avatar.png")
				content := "image"
				if name == "oversize" {
					content = strings.Repeat("x", 33)
				}
				_, _ = part.Write([]byte(content))
			case "oversize body":
				_ = writer.WriteField("username", strings.Repeat("x", (1<<20)+100))
			case "empty":
			default:
				_ = writer.WriteField("username", "new")
			}
			_ = writer.Close()
			svc := &profilePatchService{}
			if name == "provider failure" {
				svc.err = errors.New("upload user avatar failed")
			}
			if name == "database failure" {
				svc.err = errors.New("update user profile failed")
			}
			if name == "empty" {
				svc.err = service.ErrEmptyProfilePatch
			}
			h := NewAuthHandler(svc, fakeObjectURLSigner{}, config.ProfileUploadConfig{MaxBytes: 32})
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			if name != "unauthenticated" {
				c.Set(currentUserIDKey, "trusted")
			}
			c.Request = httptest.NewRequest("PATCH", "/api/v1/me/profile", body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			if name == "malformed" {
				c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
			}
			h.PatchMyProfile(c)
			want := 200
			switch name {
			case "duplicate", "unknown", "wrong file", "malformed", "empty":
				want = 400
			case "oversize", "oversize body":
				want = 413
			case "unauthenticated":
				want = 401
			case "provider failure":
				want = 502
			case "database failure":
				want = 500
			}
			if w.Code != want {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if want == 200 {
				if svc.calls != 1 || svc.userID != "trusted" {
					t.Fatal("missing trusted identity")
				}
				if name == "clear birthday" && (svc.input.Birthday == nil || *svc.input.Birthday != "" || svc.input.Username != nil) {
					t.Fatal("lost field presence")
				}
				if name == "name" && (svc.input.Username == nil || svc.input.Birthday != nil || svc.input.File != nil) {
					t.Fatal("unexpected submitted fields")
				}
				if name == "avatar" && svc.input.File == nil {
					t.Fatal("file missing")
				}
			} else if name != "provider failure" && name != "database failure" && name != "empty" && svc.calls != 0 {
				t.Fatal("invalid request reached service")
			}
		})
	}
}
