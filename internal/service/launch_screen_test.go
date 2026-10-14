package service

import (
	"context"
	"errors"
	"testing"

	"pack_mate/internal/domain"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type fakeLaunchScreenRepository struct {
	config *domain.LaunchScreen
	err    error
}

// GetActive returns the configured launch screen for service tests.
func (r *fakeLaunchScreenRepository) GetActive(context.Context) (*domain.LaunchScreen, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.config == nil {
		return nil, mongo.ErrNoDocuments
	}
	copy := *r.config
	return &copy, nil
}

type fakeLaunchScreenSigner struct {
	err  error
	keys []string
}

// SignGetURL records the object key and returns a deterministic URL.
func (s *fakeLaunchScreenSigner) SignGetURL(_ context.Context, key string) (string, error) {
	s.keys = append(s.keys, key)
	if s.err != nil {
		return "", s.err
	}
	return "https://signed.example/" + key, nil
}

// TestLaunchScreenServiceCoversImageVideoAndFallback verifies media validation and signing.
func TestLaunchScreenServiceCoversImageVideoAndFallback(t *testing.T) {
	tests := []struct {
		name       string
		config     *domain.LaunchScreen
		wantErr    bool
		wantKeys   int
		wantType   domain.LaunchScreenType
		wantPoster string
	}{
		{name: "missing config", wantKeys: 0},
		{name: "disabled", config: &domain.LaunchScreen{Enabled: false}, wantKeys: 0},
		{name: "image", config: &domain.LaunchScreen{Enabled: true, Revision: "2026-10-08", Type: domain.LaunchScreenTypeImage, MediaKey: "image.webp"}, wantKeys: 1, wantType: domain.LaunchScreenTypeImage},
		{name: "video", config: &domain.LaunchScreen{Enabled: true, Revision: "2026-10-08-02", Type: domain.LaunchScreenTypeVideo, MediaKey: "video.mp4", PosterKey: "poster.webp"}, wantKeys: 2, wantType: domain.LaunchScreenTypeVideo, wantPoster: "https://signed.example/poster.webp"},
		{name: "invalid type", config: &domain.LaunchScreen{Enabled: true, Type: "audio", MediaKey: "audio.mp3"}, wantErr: true},
		{name: "video poster required", config: &domain.LaunchScreen{Enabled: true, Type: domain.LaunchScreenTypeVideo, MediaKey: "video.mp4"}, wantErr: true},
		{name: "invalid revision", config: &domain.LaunchScreen{Enabled: true, Revision: "1.2", Type: domain.LaunchScreenTypeImage, MediaKey: "image.webp"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer := &fakeLaunchScreenSigner{}
			service := NewLaunchScreenService(&fakeLaunchScreenRepository{config: tt.config}, signer)
			got, err := service.Get(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got == nil || len(signer.keys) != tt.wantKeys {
				t.Fatalf("unexpected result: %+v, keys=%v", got, signer.keys)
			}
			if got.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", got.Type, tt.wantType)
			}
			if tt.wantType == domain.LaunchScreenTypeVideo && got.PosterKey != tt.wantPoster {
				t.Fatalf("poster = %q, want %q", got.PosterKey, tt.wantPoster)
			}
			if tt.name == "image" && got.DurationMS <= 0 {
				t.Fatal("image duration was not defaulted")
			}
		})
	}
}

// TestLaunchScreenServiceWrapsRepositoryAndSignerErrors preserves failure context.
func TestLaunchScreenServiceWrapsRepositoryAndSignerErrors(t *testing.T) {
	repositoryErr := errors.New("mongo unavailable")
	service := NewLaunchScreenService(&fakeLaunchScreenRepository{err: repositoryErr}, &fakeLaunchScreenSigner{})
	if err := serviceError(service); !errors.Is(err, repositoryErr) {
		t.Fatalf("repository error = %v", err)
	}

	signerErr := errors.New("oss unavailable")
	service = NewLaunchScreenService(&fakeLaunchScreenRepository{config: &domain.LaunchScreen{Enabled: true, Revision: "2026-10-08", Type: domain.LaunchScreenTypeImage, MediaKey: "image.webp"}}, &fakeLaunchScreenSigner{err: signerErr})
	if err := serviceError(service); !errors.Is(err, signerErr) {
		t.Fatalf("signer error = %v", err)
	}
}

func serviceError(service LaunchScreenService) error {
	_, err := service.Get(context.Background())
	return err
}
