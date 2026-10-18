package repository

import (
	"pack_mate/internal/domain"
	"time"
)

// UserProfilePatch contains typed optional profile changes without storage-specific fields.
type UserProfilePatch struct {
	Username               *string
	Gender                 *domain.UserGender
	BirthdaySet            bool
	Birthday               *time.Time
	AvatarObjectKey        *string
	AvatarDisplayObjectKey *string
}
