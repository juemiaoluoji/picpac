package mongodb

import (
	"context"
	"fmt"
	"time"

	"pack_mate/internal/config"
	"pack_mate/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Connection wraps the MongoDB client and selected database handle.
type Connection struct {
	Client   *mongo.Client
	Database *mongo.Database
}

// New creates a MongoDB client, verifies connectivity, and returns the selected database handle.
func New(cfg config.MongoConfig) (*Connection, error) {
	clientOptions := options.Client().ApplyURI(cfg.URI)
	if cfg.MaxPoolSize > 0 {
		clientOptions.SetMaxPoolSize(cfg.MaxPoolSize)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ConnectTimeoutSeconds)*time.Second)
	defer cancel()

	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	db := client.Database(cfg.Database)
	if err := ensureIndexes(ctx, db); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ensure mongo indexes: %w", err)
	}

	return &Connection{
		Client:   client,
		Database: db,
	}, nil
}

// Close disconnects the MongoDB client.
func (c *Connection) Close(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Disconnect(ctx)
}

func ensureIndexes(ctx context.Context, db *mongo.Database) error {
	if _, err := db.Collection(categoryCollectionName).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "key", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}

	if err := ensureAuthIdentityIndexes(ctx, db); err != nil {
		return err
	}

	if _, err := db.Collection(refreshTokenCollectionName).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "th", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}

	if _, err := db.Collection(userPasswordCredentialCollectionName).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "uid", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}

	return nil
}

func ensureAuthIdentityIndexes(ctx context.Context, db *mongo.Database) error {
	collection := db.Collection(authIdentityCollectionName)
	indexes := collection.Indexes()
	const indexName = "auth_identity_active_provider_identifier_unique"

	cursor, err := indexes.List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var index struct {
			Name                    string `bson:"name"`
			Key                     bson.D `bson:"key"`
			Unique                  bool   `bson:"unique"`
			PartialFilterExpression bson.M `bson:"partialFilterExpression"`
		}
		if err := cursor.Decode(&index); err != nil {
			return err
		}
		if !isAuthIdentityProviderIdentifierIndex(index.Key) {
			continue
		}
		if isActiveAuthIdentityUniqueIndex(index.Name, index.Unique, index.PartialFilterExpression) {
			return nil
		}
		if err := indexes.DropOne(ctx, index.Name); err != nil {
			return err
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}

	_, err = indexes.CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "prv", Value: 1},
			{Key: "idf", Value: 1},
		},
		Options: options.Index().
			SetName(indexName).
			SetUnique(true).
			SetPartialFilterExpression(bson.M{"st": domain.AuthIdentityStatusActive}),
	})
	return err
}

func isAuthIdentityProviderIdentifierIndex(key bson.D) bool {
	return len(key) == 2 &&
		key[0].Key == "prv" && fmt.Sprint(key[0].Value) == "1" &&
		key[1].Key == "idf" && fmt.Sprint(key[1].Value) == "1"
}

func isActiveAuthIdentityUniqueIndex(name string, unique bool, partial bson.M) bool {
	if name != "auth_identity_active_provider_identifier_unique" || !unique {
		return false
	}
	return fmt.Sprint(partial["st"]) == string(domain.AuthIdentityStatusActive)
}
