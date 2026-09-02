package persistence

import (
	"context"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/pocketbase/dbx"
)

type apiKeyRepository struct {
	sqlRepository
}

func NewAPIKeyRepository(ctx context.Context, db dbx.Builder) model.UserAPIKeyRepository {
	r := &apiKeyRepository{}
	r.ctx = ctx
	r.db = db
	r.tableName = "user_api_key"
	r.registerModel(&model.UserAPIKey{}, nil)
	return r
}

func (r *apiKeyRepository) Get(id string) (*model.UserAPIKey, error) {
	sel := r.newSelect().Where(Eq{"id": id})
	var key model.UserAPIKey
	if err := r.queryOne(sel, &key); err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *apiKeyRepository) GetByUserID(userID string) (model.UserAPIKeys, error) {
	sel := r.newSelect().
		Where(Eq{"user_id": userID}).
		OrderBy("created_at DESC")
	var keys model.UserAPIKeys
	if err := r.queryAll(sel, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *apiKeyRepository) FindByLookupPrefix(prefix string) (model.UserAPIKeys, error) {
	sel := r.newSelect().Where(Eq{"lookup_prefix": prefix})
	var keys model.UserAPIKeys
	if err := r.queryAll(sel, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *apiKeyRepository) Put(key *model.UserAPIKey) error {
	_, err := r.put(key.ID, key)
	return err
}

func (r *apiKeyRepository) Delete(id string) error {
	return r.delete(Eq{"id": id})
}

func (r *apiKeyRepository) TouchLastUsed(id string, usedAt time.Time) error {
	update := Update(r.tableName).
		Set("last_used_at", usedAt).
		Set("updated_at", usedAt).
		Where(Eq{"id": id})
	_, err := r.executeSQL(update)
	return err
}
