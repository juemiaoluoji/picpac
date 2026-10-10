package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// LaunchScreenType identifies the media type used by a launch screen.
type LaunchScreenType string

const (
	LaunchScreenTypeImage LaunchScreenType = "image"
	LaunchScreenTypeVideo LaunchScreenType = "video"
)

// LaunchScreen contains the active launch screen configuration.
type LaunchScreen struct {
	ID          bson.ObjectID
	Enabled     bool
	Revision    string
	Type        LaunchScreenType
	MediaKey    string
	PosterKey   string
	DurationMS  int
	SkipEnabled bool
	UpdatedAt   time.Time
}
