package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"pack_mate/internal/domain"
	"pack_mate/internal/repository"
)

// ErrAvatarTooLarge indicates an avatar exceeds configured upload limits.
var ErrAvatarTooLarge = errors.New("avatar exceeds upload limits")

// ErrEmptyProfilePatch indicates no editable profile field was submitted.
var ErrEmptyProfilePatch = errors.New("at least one profile field is required")

// ProfilePatchInput distinguishes omitted fields from explicit empty values.
type ProfilePatchInput struct {
	Username *string
	Gender   *string
	Birthday *string
	File     io.ReadSeeker
	FileName string
}

// PatchMyProfile validates and updates only explicitly supplied profile fields.
func (s *authService) PatchMyProfile(ctx context.Context, userID string, input ProfilePatchInput) (*domain.User, error) {
	id, err := parseObjectID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid input")
	}
	if input.Username == nil && input.Gender == nil && input.Birthday == nil && input.File == nil {
		return nil, ErrEmptyProfilePatch
	}
	if _, err := s.getActiveUserByID(ctx, id); err != nil {
		return nil, err
	}
	patch := repository.UserProfilePatch{}
	if input.Username != nil {
		name := strings.TrimSpace(*input.Username)
		if name == "" {
			return nil, fmt.Errorf("username is required")
		}
		if len([]rune(name)) > 32 {
			return nil, fmt.Errorf("username is too long")
		}
		patch.Username = &name
	}
	if input.Gender != nil {
		gender, err := parseRequiredUserGender(*input.Gender)
		if err != nil {
			return nil, err
		}
		patch.Gender = &gender
	}
	if input.Birthday != nil {
		date, err := parseOptionalBirthday(*input.Birthday)
		if err != nil {
			return nil, err
		}
		patch.BirthdaySet = true
		patch.Birthday = date
	}
	if input.File != nil {
		// Bound reads and inspect dimensions before allocating decoded pixels.
		if _, err := input.File.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("invalid input")
		}
		limits := s.profileUpload.WithDefaults()
		body, err := io.ReadAll(io.LimitReader(input.File, limits.MaxBytes+1))
		if err != nil {
			return nil, fmt.Errorf("invalid input")
		}
		if int64(len(body)) > limits.MaxBytes {
			return nil, ErrAvatarTooLarge
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			return nil, fmt.Errorf("invalid input")
		}
		if int64(cfg.Width) > limits.MaxPixels/int64(cfg.Height) {
			return nil, ErrAvatarTooLarge
		}
		source, display, err := s.uploadUserAvatar(ctx, id, input.FileName, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		patch.AvatarObjectKey = &source
		patch.AvatarDisplayObjectKey = &display
	}
	user, err := s.users.PatchProfile(ctx, id, patch, time.Now().UTC())
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("update user profile failed: %w", err)
	}
	return user, nil
}
