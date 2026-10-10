package repository

import (
	"context"
	"time"

	"pack_mate/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CategoryRepository defines persistence behavior for system categories.
type CategoryRepository interface {
	ListAll(ctx context.Context) ([]domain.Category, error)
	GetByID(ctx context.Context, categoryID bson.ObjectID) (*domain.Category, error)
	GetByKey(ctx context.Context, key string) (*domain.Category, error)
	UpsertByKey(ctx context.Context, category *domain.Category) error
}

// LaunchScreenRepository defines persistence behavior for launch screen configuration.
type LaunchScreenRepository interface {
	GetActive(ctx context.Context) (*domain.LaunchScreen, error)
}

// ItemRepository defines persistence behavior for items.
type ItemRepository interface {
	Create(ctx context.Context, item *domain.Item) error
	CreateMany(ctx context.Context, items []domain.Item) error
	ListAll(ctx context.Context) ([]domain.Item, error)
	ListByUserID(ctx context.Context, userID bson.ObjectID) ([]domain.Item, error)
	ListByFilter(ctx context.Context, filter ItemFilter) ([]domain.Item, error)
	SearchByKeyword(ctx context.Context, keyword string) ([]domain.Item, error)
	SearchByKeywordAndUserID(ctx context.Context, userID bson.ObjectID, keyword string) ([]domain.Item, error)
	GetByID(ctx context.Context, itemID bson.ObjectID) (*domain.Item, error)
	Update(ctx context.Context, item *domain.Item) error
	DeleteByID(ctx context.Context, itemID bson.ObjectID) error
}

// ItemFilter defines item list filters.
type ItemFilter struct {
	UserID        bson.ObjectID
	Keyword       string
	HasKeyword    bool
	CategoryID    bson.ObjectID
	HasCategoryID bool
}

// PackRepository defines persistence behavior for packs.
type PackRepository interface {
	Create(ctx context.Context, pack *domain.Pack) error
	ListAll(ctx context.Context) ([]domain.Pack, error)
	ListByUserID(ctx context.Context, userID bson.ObjectID) ([]domain.Pack, error)
	SearchByKeyword(ctx context.Context, keyword string) ([]domain.Pack, error)
	SearchByKeywordAndUserID(ctx context.Context, userID bson.ObjectID, keyword string) ([]domain.Pack, error)
	GetByID(ctx context.Context, packID bson.ObjectID) (*domain.Pack, error)
	Update(ctx context.Context, pack *domain.Pack) error
	DeleteByID(ctx context.Context, packID bson.ObjectID) error
}

// ChecklistRepository defines persistence behavior for checklists.
type ChecklistRepository interface {
	Create(ctx context.Context, checklist *domain.Checklist) error
	ListAll(ctx context.Context) ([]domain.Checklist, error)
	ListByUserID(ctx context.Context, userID bson.ObjectID) ([]domain.Checklist, error)
	SearchByKeyword(ctx context.Context, keyword string) ([]domain.Checklist, error)
	SearchByKeywordAndUserID(ctx context.Context, userID bson.ObjectID, keyword string) ([]domain.Checklist, error)
	GetByID(ctx context.Context, checklistID bson.ObjectID) (*domain.Checklist, error)
	Update(ctx context.Context, checklist *domain.Checklist) error
	UpdateLineItemStatus(ctx context.Context, checklistID bson.ObjectID, lineItemID bson.ObjectID, status domain.LineItemStatus, updatedAt time.Time) error
	DeleteByID(ctx context.Context, checklistID bson.ObjectID) error
}

// UserRepository defines persistence behavior for users.
type UserRepository interface {
	PatchProfile(ctx context.Context, userID bson.ObjectID, patch UserProfilePatch, updatedAt time.Time) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, userID bson.ObjectID) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
}

// AuthIdentityRepository defines persistence behavior for auth identities.
type AuthIdentityRepository interface {
	Create(ctx context.Context, identity *domain.AuthIdentity) error
	GetByProviderAndIdentifier(ctx context.Context, provider domain.AuthProvider, identifier string) (*domain.AuthIdentity, error)
	GetByUserIDAndProvider(ctx context.Context, userID bson.ObjectID, provider domain.AuthProvider) (*domain.AuthIdentity, error)
	DisableByUserID(ctx context.Context, userID bson.ObjectID, disabledAt time.Time) error
}

// RefreshTokenRepository defines persistence behavior for refresh tokens.
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	Revoke(ctx context.Context, tokenHash string, revokedAt time.Time) error
	RevokeByUserID(ctx context.Context, userID bson.ObjectID, revokedAt time.Time) error
}

// UserPasswordCredentialRepository defines persistence behavior for password credentials.
type UserPasswordCredentialRepository interface {
	UpdatePasswordAndRevokeTokens(ctx context.Context, userID bson.ObjectID, expectedHash, passwordHash, passwordAlgo string, updatedAt time.Time) error
	Create(ctx context.Context, credential *domain.UserPasswordCredential) error
	GetByUserID(ctx context.Context, userID bson.ObjectID) (*domain.UserPasswordCredential, error)
	RecordFailure(ctx context.Context, userID bson.ObjectID, failedAttemptCount int, lockedUntil *time.Time, updatedAt time.Time) error
	RecordSuccess(ctx context.Context, userID bson.ObjectID, usedAt time.Time) error
	UpdatePassword(ctx context.Context, userID bson.ObjectID, passwordHash string, passwordAlgo string, updatedAt time.Time) error
}
