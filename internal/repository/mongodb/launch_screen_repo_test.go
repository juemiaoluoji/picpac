package mongodb

import (
	"testing"
	"time"

	"pack_mate/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestLaunchScreenDocumentMapping preserves launch screen media configuration across the repository boundary.
func TestLaunchScreenDocumentMapping(t *testing.T) {
	now := time.Now().UTC()
	id := bson.NewObjectID()
	doc := launchScreenDocument{
		ID: id, Enabled: true, Revision: "2026-10-08-02", Type: "video",
		MediaKey: "app/launch-screen/v4/video.mp4", PosterKey: "app/launch-screen/v4/poster.webp",
		DurationMS: 5000, SkipEnabled: true, UpdatedAt: now,
	}
	config := domain.LaunchScreen{
		ID: doc.ID, Enabled: doc.Enabled, Revision: doc.Revision,
		Type: domain.LaunchScreenType(doc.Type), MediaKey: doc.MediaKey, PosterKey: doc.PosterKey,
		DurationMS: doc.DurationMS, SkipEnabled: doc.SkipEnabled, UpdatedAt: doc.UpdatedAt,
	}
	if config.ID != id || config.Type != domain.LaunchScreenTypeVideo || config.MediaKey != doc.MediaKey || config.PosterKey != doc.PosterKey || !config.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected launch screen mapping: %+v", config)
	}
}
