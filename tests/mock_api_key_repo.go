package tests

import (
	"time"

	"github.com/navidrome/navidrome/model"
)

type MockAPIKeyRepo struct {
	keys map[string]model.UserAPIKey
}

func (m *MockAPIKeyRepo) Get(id string) (*model.UserAPIKey, error) {
	key, ok := m.keys[id]
	if !ok {
		return nil, model.ErrNotFound
	}
	return &key, nil
}

func (m *MockAPIKeyRepo) GetByUserID(userID string) (model.UserAPIKeys, error) {
	var keys model.UserAPIKeys
	for _, key := range m.keys {
		if key.UserID == userID {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *MockAPIKeyRepo) FindByLookupPrefix(prefix string) (model.UserAPIKeys, error) {
	var keys model.UserAPIKeys
	for _, key := range m.keys {
		if key.LookupPrefix == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (m *MockAPIKeyRepo) Put(key *model.UserAPIKey) error {
	m.keys[key.ID] = *key
	return nil
}

func (m *MockAPIKeyRepo) Delete(id string) error {
	delete(m.keys, id)
	return nil
}

func (m *MockAPIKeyRepo) TouchLastUsed(id string, usedAt time.Time) error {
	key, ok := m.keys[id]
	if !ok {
		return model.ErrNotFound
	}
	key.LastUsedAt = &usedAt
	m.keys[id] = key
	return nil
}
