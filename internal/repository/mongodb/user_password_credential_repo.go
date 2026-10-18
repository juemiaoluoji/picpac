package mongodb

import (
	"context"
	"time"

	"pack_mate/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const userPasswordCredentialCollectionName = "user_password_credentials"

type userPasswordCredentialDocument struct {
	ID                 bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID             bson.ObjectID `json:"user_id" bson:"uid"`
	PasswordHash       string        `json:"password_hash" bson:"ph"`
	PasswordAlgo       string        `json:"password_algo" bson:"pa"`
	FailedAttemptCount int           `json:"failed_attempt_count" bson:"fac"`
	LockedUntil        *time.Time    `json:"locked_until" bson:"lck,omitempty"`
	LastUsedAt         *time.Time    `json:"last_used_at" bson:"lut,omitempty"`
	CreatedAt          time.Time     `json:"created_at" bson:"cat"`
	UpdatedAt          time.Time     `json:"updated_at" bson:"uat"`
}

// UserPasswordCredentialRepository stores user password credentials in MongoDB.
type UserPasswordCredentialRepository struct {
	collection *mongo.Collection
	database   *mongo.Database
}

// NewUserPasswordCredentialRepository creates a MongoDB-backed password credential repository.
func NewUserPasswordCredentialRepository(db *mongo.Database) *UserPasswordCredentialRepository {
	return &UserPasswordCredentialRepository{collection: db.Collection(userPasswordCredentialCollectionName), database: db}
}

// Create inserts a user password credential document into MongoDB.
func (r *UserPasswordCredentialRepository) Create(ctx context.Context, credential *domain.UserPasswordCredential) error {
	_, err := r.collection.InsertOne(ctx, newUserPasswordCredentialDocument(credential))
	return err
}

// GetByUserID returns a user password credential by user id.
func (r *UserPasswordCredentialRepository) GetByUserID(ctx context.Context, userID bson.ObjectID) (*domain.UserPasswordCredential, error) {
	var doc userPasswordCredentialDocument
	if err := r.collection.FindOne(ctx, bson.M{"uid": userID}).Decode(&doc); err != nil {
		return nil, err
	}

	credential := newDomainUserPasswordCredential(doc)
	return &credential, nil
}

// RecordFailure records a failed password login attempt.
func (r *UserPasswordCredentialRepository) RecordFailure(ctx context.Context, userID bson.ObjectID, failedAttemptCount int, lockedUntil *time.Time, updatedAt time.Time) error {
	set := bson.M{
		"fac": failedAttemptCount,
		"uat": updatedAt,
	}
	if lockedUntil != nil {
		set["lck"] = lockedUntil
	}

	update := bson.M{"$set": set}
	if lockedUntil == nil {
		update["$unset"] = bson.M{"lck": ""}
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"uid": userID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// RecordSuccess records a successful password login attempt.
func (r *UserPasswordCredentialRepository) RecordSuccess(ctx context.Context, userID bson.ObjectID, usedAt time.Time) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{"uid": userID}, bson.M{
		"$set": bson.M{
			"fac": 0,
			"lut": usedAt,
			"uat": usedAt,
		},
		"$unset": bson.M{
			"lck": "",
		},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// UpdatePassword updates a user's password hash and clears password failure state.
func (r *UserPasswordCredentialRepository) UpdatePassword(ctx context.Context, userID bson.ObjectID, passwordHash string, passwordAlgo string, updatedAt time.Time) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{"uid": userID}, bson.M{
		"$set": bson.M{
			"ph":  passwordHash,
			"pa":  passwordAlgo,
			"fac": 0,
			"uat": updatedAt,
		},
		"$unset": bson.M{
			"lck": "",
		},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

func newUserPasswordCredentialDocument(credential *domain.UserPasswordCredential) userPasswordCredentialDocument {
	return userPasswordCredentialDocument{
		ID:                 credential.ID,
		UserID:             credential.UserID,
		PasswordHash:       credential.PasswordHash,
		PasswordAlgo:       credential.PasswordAlgo,
		FailedAttemptCount: credential.FailedAttemptCount,
		LockedUntil:        credential.LockedUntil,
		LastUsedAt:         credential.LastUsedAt,
		CreatedAt:          credential.CreatedAt,
		UpdatedAt:          credential.UpdatedAt,
	}
}

func newDomainUserPasswordCredential(doc userPasswordCredentialDocument) domain.UserPasswordCredential {
	return domain.UserPasswordCredential{
		ID:                 doc.ID,
		UserID:             doc.UserID,
		PasswordHash:       doc.PasswordHash,
		PasswordAlgo:       doc.PasswordAlgo,
		FailedAttemptCount: doc.FailedAttemptCount,
		LockedUntil:        doc.LockedUntil,
		LastUsedAt:         doc.LastUsedAt,
		CreatedAt:          doc.CreatedAt,
		UpdatedAt:          doc.UpdatedAt,
	}
}

// UpdatePasswordAndRevokeTokens atomically replaces an unchanged password and revokes existing refresh tokens.
func (r *UserPasswordCredentialRepository) UpdatePasswordAndRevokeTokens(ctx context.Context, userID bson.ObjectID, expectedHash, passwordHash, passwordAlgo string, updatedAt time.Time) error {
	session, err := r.database.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		result, err := r.collection.UpdateOne(tx, bson.M{"uid": userID, "ph": expectedHash}, bson.M{
			"$set":   bson.M{"ph": passwordHash, "pa": passwordAlgo, "fac": 0, "uat": updatedAt},
			"$unset": bson.M{"lck": ""},
		})
		if err != nil {
			return nil, err
		}
		if result.MatchedCount == 0 {
			return nil, mongo.ErrNoDocuments
		}
		err = NewRefreshTokenRepository(r.database).RevokeByUserID(tx, userID, updatedAt)
		return nil, err
	})
	return err
}
