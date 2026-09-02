package model

import "time"

type UserAPIKey struct {
	ID           string     `structs:"id" json:"id"`
	UserID       string     `structs:"user_id" json:"userId"`
	Name         string     `structs:"name" json:"name"`
	LookupPrefix string     `structs:"lookup_prefix" json:"-"`
	TokenHash    string     `structs:"token_hash" json:"-"`
	LastUsedAt   *time.Time `structs:"last_used_at" json:"lastUsedAt,omitempty"`
	ExpiresAt    *time.Time `structs:"expires_at" json:"expiresAt,omitempty"`
	CreatedAt    time.Time  `structs:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `structs:"updated_at" json:"updatedAt"`
}

type UserAPIKeys []UserAPIKey

type UserAPIKeyRepository interface {
	Get(id string) (*UserAPIKey, error)
	GetByUserID(userID string) (UserAPIKeys, error)
	FindByLookupPrefix(prefix string) (UserAPIKeys, error)
	Put(*UserAPIKey) error
	Delete(id string) error
	TouchLastUsed(id string, usedAt time.Time) error
}
