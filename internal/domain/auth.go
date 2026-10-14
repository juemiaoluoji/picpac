package domain

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type AuthIdentity struct {
	ID         bson.ObjectID      `json:"id"`
	UserID     bson.ObjectID      `json:"user_id"`
	Provider   AuthProvider       `json:"provider"`
	Identifier string             `json:"identifier"` // phone number or wechat id
	Status     AuthIdentityStatus `json:"status"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

type AuthProvider string

const (
	AuthProviderPhone  AuthProvider = "phone"
	AuthProviderWechat AuthProvider = "wechat"
)

type AuthIdentityStatus string

const (
	AuthIdentityStatusActive   AuthIdentityStatus = "active"
	AuthIdentityStatusDisabled AuthIdentityStatus = "disabled"
)

type RefreshToken struct {
	ID        bson.ObjectID `json:"id"`
	UserID    bson.ObjectID `json:"user_id"`
	TokenHash string        `json:"token_hash"`
	ExpiresAt time.Time     `json:"expires_at"`
	RevokedAt *time.Time    `json:"revoked_at"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type AuthSecurity struct {
	Phone         string `json:"phone"`
	PasswordSetup bool   `json:"password_setup"`
}

type UserPasswordCredential struct {
	ID                 bson.ObjectID `json:"id"`
	UserID             bson.ObjectID `json:"user_id"`
	PasswordHash       string        `json:"password_hash"`
	PasswordAlgo       string        `json:"password_algo"`
	FailedAttemptCount int           `json:"failed_attempt_count"`
	LockedUntil        *time.Time    `json:"locked_until"`
	LastUsedAt         *time.Time    `json:"last_used_at"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}
