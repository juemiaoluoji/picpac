package service

import (
	"bytes"
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"image"
	"image/png"
	"pack_mate/internal/domain"
	"pack_mate/internal/dto/request"
	"pack_mate/internal/repository"
	"testing"
	"time"
)

// PatchProfile applies supplied fields to the fake user's profile.
func (r *fakeUserRepository) PatchProfile(_ context.Context, _ bson.ObjectID, p repository.UserProfilePatch, at time.Time) (*domain.User, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.got == nil {
		return nil, mongo.ErrNoDocuments
	}
	u := *r.got
	if p.Username != nil {
		u.Profile.Username = *p.Username
	}
	if p.Gender != nil {
		u.Profile.Gender = *p.Gender
	}
	if p.BirthdaySet {
		u.Profile.Birthday = p.Birthday
	}
	if p.AvatarObjectKey != nil {
		u.Profile.AvatarObjectKey = *p.AvatarObjectKey
	}
	if p.AvatarDisplayObjectKey != nil {
		u.Profile.AvatarDisplayObjectKey = *p.AvatarDisplayObjectKey
	}
	u.UpdatedAt = at
	r.got = &u
	r.updated = &u
	return &u, nil
}

// TestProfilePatchPreservesOmittedFields checks omitted, cleared and invalid values.
func TestProfilePatchPreservesOmittedFields(t *testing.T) {
	for _, name := range []string{"username", "clear birthday", "set birthday", "empty", "invalid name", "invalid gender", "invalid birthday"} {
		t.Run(name, func(t *testing.T) {
			date := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			users := &fakeUserRepository{got: &domain.User{ID: bson.NewObjectID(), Status: domain.UserStatusCreated, Profile: domain.UserProfile{Username: "old", Gender: domain.UserGenderPrivate, Birthday: &date, AvatarObjectKey: defaultUserAvatarObjectKey}}}
			svc := newTestAuthService(users, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, &recordingPhoneVerificationService{})
			input := ProfilePatchInput{}
			value := ""
			switch name {
			case "username":
				value = " new "
				input.Username = &value
			case "clear birthday":
				input.Birthday = &value
			case "set birthday":
				value = "2008-01-01"
				input.Birthday = &value
			case "invalid name":
				input.Username = &value
			case "invalid gender":
				value = "bad"
				input.Gender = &value
			case "invalid birthday":
				value = "bad"
				input.Birthday = &value
			}
			user, err := svc.PatchMyProfile(context.Background(), users.got.ID.Hex(), input)
			good := name == "username" || name == "clear birthday" || name == "set birthday"
			if (err == nil) != good {
				t.Fatalf("unexpected error %v", err)
			}
			if !good {
				if users.updated != nil {
					t.Fatal("invalid patch wrote data")
				}
				return
			}
			if user.Profile.AvatarObjectKey != defaultUserAvatarObjectKey || user.Profile.Gender != domain.UserGenderPrivate {
				t.Fatal("omitted fields changed")
			}
			if name == "username" && (user.Profile.Username != "new" || !user.Profile.Birthday.Equal(date)) {
				t.Fatal("username patch changed birthday")
			}
			if name == "clear birthday" && user.Profile.Birthday != nil {
				t.Fatal("birthday not cleared")
			}
			if name == "set birthday" && user.Profile.Birthday.Format("2006-01-02") != "2008-01-01" {
				t.Fatal("birthday not set")
			}
		})
	}
}

// TestDefaultUsername uses only the last four phone digits.
func TestDefaultUsername(t *testing.T) {
	for _, phone := range []string{"13800138000", "+8613800138000"} {
		if got := newDefaultUsername(phone); got != "picpacker_8000" {
			t.Fatalf("got %s", got)
		}
	}
}

// TestProfilePatchUploadFailures ensures validation precedes uploads and failed uploads do not persist data.
func TestProfilePatchUploadFailures(t *testing.T) {
	for _, name := range []string{"text only", "avatar only", "bad image", "large bytes", "large pixels", "bad name with image", "upload failure", "database failure", "disabled", "deleted"} {
		t.Run(name, func(t *testing.T) {
			users := &fakeUserRepository{got: &domain.User{ID: bson.NewObjectID(), Status: domain.UserStatusCreated, Profile: domain.UserProfile{Username: "old", AvatarObjectKey: defaultUserAvatarObjectKey}}}
			uploader := &fakeAuthUploadService{}
			cfg := validAuthConfig()
			input := ProfilePatchInput{File: testImageReader(), FileName: "avatar.png"}
			value := "new"
			switch name {
			case "text only":
				input.File = nil
				input.Username = &value
			case "bad image":
				input.File = bytes.NewReader([]byte("bad"))
			case "large bytes":
				cfg.ProfileUpload.MaxBytes = 1
			case "large pixels":
				var b bytes.Buffer
				if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
					t.Fatal(err)
				}
				input.File = bytes.NewReader(b.Bytes())
				cfg.ProfileUpload.MaxPixels = 1
			case "bad name with image":
				value = ""
				input.Username = &value
			case "upload failure":
				uploader.err = errors.New("storage down")
			case "database failure":
				users.updateErr = errors.New("db down")
			case "disabled":
				users.got.Status = domain.UserStatusDisabled
			case "deleted":
				users.got.Status = domain.UserStatusDeleted
			}
			svc := NewAuthService(users, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, &fakeUserPasswordCredentialRepository{}, uploader, &recordingPhoneVerificationService{}, NewTokenService(cfg.AccessTokenSecret, time.Hour), cfg, "dev")
			_, err := svc.PatchMyProfile(context.Background(), users.got.ID.Hex(), input)
			success := name == "text only" || name == "avatar only"
			if (err == nil) != success {
				t.Fatalf("unexpected error: %v", err)
			}
			if !success && users.updated != nil {
				t.Fatal("persisted failed patch")
			}
			if name != "avatar only" && name != "database failure" && len(uploader.objectKeys) > 0 {
				t.Fatal("unexpected upload")
			}
			if name == "avatar only" && len(uploader.objectKeys) != 2 {
				t.Fatal("missing source/display uploads")
			}
			if (name == "large bytes" || name == "large pixels") && !errors.Is(err, ErrAvatarTooLarge) {
				t.Fatal("size error missing")
			}
		})
	}
}

// TestLegacyProfileUpdateClearsOmittedBirthday preserves the original PUT semantics.
func TestLegacyProfileUpdateClearsOmittedBirthday(t *testing.T) {
	date := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	users := &fakeUserRepository{got: &domain.User{ID: bson.NewObjectID(), Status: domain.UserStatusCreated, Profile: domain.UserProfile{Username: "old", Birthday: &date, AvatarObjectKey: defaultUserAvatarObjectKey}}}
	svc := newTestAuthService(users, &fakeAuthIdentityRepository{}, &fakeRefreshTokenRepository{}, &recordingPhoneVerificationService{})
	user, err := svc.UpdateMyProfile(context.Background(), users.got.ID.Hex(), request.UpdateMyProfileInput{Username: "new", Gender: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if user.Profile.Birthday != nil {
		t.Fatal("legacy PUT did not clear omitted birthday")
	}
	if user.Profile.AvatarObjectKey != defaultUserAvatarObjectKey {
		t.Fatal("legacy PUT lost avatar")
	}
}
