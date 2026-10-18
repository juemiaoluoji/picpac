package mongodb

import (
	"context"
	"time"

	"pack_mate/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const launchScreenCollectionName = "launch_screen_configs"

type launchScreenDocument struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Enabled     bool          `json:"enabled" bson:"ena"`
	Revision    string        `json:"revision" bson:"rev"`
	Type        string        `json:"type" bson:"typ"`
	MediaKey    string        `json:"media_key" bson:"mk"`
	PosterKey   string        `json:"poster_key" bson:"pk,omitempty"`
	DurationMS  int           `json:"duration_ms" bson:"dur"`
	SkipEnabled bool          `json:"skip_enabled" bson:"skip"`
	UpdatedAt   time.Time     `json:"updated_at" bson:"uat"`
}

// LaunchScreenRepository stores launch screen configuration in MongoDB.
type LaunchScreenRepository struct {
	collection *mongo.Collection
}

// NewLaunchScreenRepository creates a MongoDB-backed launch screen repository.
func NewLaunchScreenRepository(db *mongo.Database) *LaunchScreenRepository {
	return &LaunchScreenRepository{collection: db.Collection(launchScreenCollectionName)}
}

// GetActive returns the single configured launch screen.
func (r *LaunchScreenRepository) GetActive(ctx context.Context) (*domain.LaunchScreen, error) {
	var doc launchScreenDocument
	if err := r.collection.FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "rev", Value: -1}})).Decode(&doc); err != nil {
		return nil, err
	}
	config := domain.LaunchScreen{
		ID: doc.ID, Enabled: doc.Enabled, Revision: doc.Revision,
		Type: domain.LaunchScreenType(doc.Type), MediaKey: doc.MediaKey,
		PosterKey: doc.PosterKey, DurationMS: doc.DurationMS,
		SkipEnabled: doc.SkipEnabled, UpdatedAt: doc.UpdatedAt,
	}
	return &config, nil
}
