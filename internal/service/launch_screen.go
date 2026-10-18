package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"pack_mate/internal/domain"
	"pack_mate/internal/repository"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

const (
	defaultLaunchScreenDurationMS      = 2500
	defaultVideoLaunchScreenDurationMS = 5000
)

var launchScreenRevisionPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(-\d{2})?$`)

// LaunchScreenService defines launch screen configuration behavior.
type LaunchScreenService interface {
	Get(ctx context.Context) (*domain.LaunchScreen, error)
}

type launchScreenService struct {
	repo      repository.LaunchScreenRepository
	urlSigner ObjectURLSigner
}

// NewLaunchScreenService creates a launch screen service.
func NewLaunchScreenService(repo repository.LaunchScreenRepository, urlSigner ObjectURLSigner) LaunchScreenService {
	return &launchScreenService{repo: repo, urlSigner: urlSigner}
}

// Get returns the active launch screen configuration with signed media URLs.
func (s *launchScreenService) Get(ctx context.Context) (*domain.LaunchScreen, error) {
	config, err := s.repo.GetActive(ctx)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &domain.LaunchScreen{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get launch screen failed: %w", err)
	}

	if !config.Enabled {
		return config, nil
	}
	if err := validateLaunchScreen(config); err != nil {
		return nil, err
	}
	if s.urlSigner == nil {
		return nil, fmt.Errorf("launch screen URL signer is not configured")
	}

	config.MediaKey, err = s.urlSigner.SignGetURL(ctx, config.MediaKey)
	if err != nil {
		return nil, fmt.Errorf("sign launch screen media URL failed: %w", err)
	}
	if config.Type == domain.LaunchScreenTypeVideo {
		config.PosterKey, err = s.urlSigner.SignGetURL(ctx, config.PosterKey)
		if err != nil {
			return nil, fmt.Errorf("sign launch screen poster URL failed: %w", err)
		}
	}
	return config, nil
}

func validateLaunchScreen(config *domain.LaunchScreen) error {
	if config == nil {
		return fmt.Errorf("invalid launch screen configuration")
	}
	config.Type = domain.LaunchScreenType(strings.TrimSpace(string(config.Type)))
	config.MediaKey = strings.TrimSpace(config.MediaKey)
	config.PosterKey = strings.TrimSpace(config.PosterKey)
	if config.Type != domain.LaunchScreenTypeImage && config.Type != domain.LaunchScreenTypeVideo {
		return fmt.Errorf("invalid launch screen type")
	}
	if config.MediaKey == "" {
		return fmt.Errorf("launch screen media is required")
	}
	if !launchScreenRevisionPattern.MatchString(strings.TrimSpace(config.Revision)) {
		return fmt.Errorf("invalid launch screen revision")
	}
	if config.Type == domain.LaunchScreenTypeVideo && config.PosterKey == "" {
		return fmt.Errorf("launch screen poster is required")
	}
	if config.DurationMS <= 0 {
		if config.Type == domain.LaunchScreenTypeVideo {
			config.DurationMS = defaultVideoLaunchScreenDurationMS
		} else {
			config.DurationMS = defaultLaunchScreenDurationMS
		}
	}
	return nil
}
