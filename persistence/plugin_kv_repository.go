package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/pocketbase/dbx"
)

const pluginKVNotExpired = "(expires_at IS NULL OR expires_at >= datetime('now'))"

type pluginKVRepository struct {
	sqlRepository
}

func NewPluginKVRepository(ctx context.Context, db dbx.Builder) model.PluginKVRepository {
	r := &pluginKVRepository{}
	r.ctx = ctx
	r.db = db
	r.tableName = "plugin_kvstore"
	return r
}

func (r *pluginKVRepository) Get(ctx context.Context, pluginID, key string) ([]byte, bool, error) {
	sel := Select("value").From(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Eq{"key": key},
		Expr(pluginKVNotExpired),
	})
	var row struct{ Value []byte }
	err := r.withCtx(ctx).queryOne(sel, &row)
	if errors.Is(err, model.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return row.Value, true, nil
}

func (r *pluginKVRepository) GetMany(ctx context.Context, pluginID string, keys []string) (map[string][]byte, error) {
	if len(keys) == 0 {
		return map[string][]byte{}, nil
	}
	sel := Select("key", "value").From(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Eq{"key": keys},
		Expr(pluginKVNotExpired),
	})
	var rows []struct {
		Key   string
		Value []byte
	}
	if err := r.withCtx(ctx).queryAll(sel, &rows); err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func (r *pluginKVRepository) Put(ctx context.Context, pluginID, key string, value []byte, ttlSeconds int64) error {
	var ttl sql.NullString
	if ttlSeconds > 0 {
		ttl = sql.NullString{String: fmt.Sprintf("+%d seconds", ttlSeconds), Valid: true}
	}
	_, err := r.db.NewQuery(`
		INSERT INTO plugin_kvstore (plugin_id, key, value, size, created_at, updated_at, expires_at)
		VALUES ({:plugin}, {:key}, {:value}, {:size}, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, datetime('now', {:ttl}))
		ON CONFLICT(plugin_id, key) DO UPDATE SET
			value = excluded.value,
			size = excluded.size,
			updated_at = CURRENT_TIMESTAMP,
			expires_at = excluded.expires_at
	`).Bind(dbx.Params{
		"plugin": pluginID,
		"key":    key,
		"value":  value,
		"size":   int64(len(value)),
		"ttl":    ttl,
	}).WithContext(ctx).Execute()
	if err != nil {
		return fmt.Errorf("storing plugin kv value: %w", err)
	}
	return nil
}

func (r *pluginKVRepository) PutExpired(ctx context.Context, pluginID, key string, value []byte) error {
	_, err := r.db.NewQuery(`
		INSERT INTO plugin_kvstore (plugin_id, key, value, size, expires_at)
		VALUES ({:plugin}, {:key}, {:value}, {:size}, datetime('now', '-1 seconds'))
		ON CONFLICT(plugin_id, key) DO UPDATE SET
			value = excluded.value,
			size = excluded.size,
			expires_at = excluded.expires_at
	`).Bind(dbx.Params{
		"plugin": pluginID,
		"key":    key,
		"value":  value,
		"size":   int64(len(value)),
	}).WithContext(ctx).Execute()
	return err
}

func (r *pluginKVRepository) Delete(ctx context.Context, pluginID, key string) error {
	_, err := r.withCtx(ctx).executeSQL(Delete(r.tableName).Where(And{Eq{"plugin_id": pluginID}, Eq{"key": key}}))
	return err
}

func (r *pluginKVRepository) DeleteByPrefix(ctx context.Context, pluginID, prefix string) (int64, error) {
	escaped := escapeLike(prefix)
	return r.withCtx(ctx).executeSQL(Delete(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Expr("key LIKE ? ESCAPE '\\'", escaped+"%"),
	}))
}

func (r *pluginKVRepository) Has(ctx context.Context, pluginID, key string) (bool, error) {
	sel := Select("COUNT(*) as count").From(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Eq{"key": key},
		Expr(pluginKVNotExpired),
	})
	var row struct{ Count int }
	if err := r.withCtx(ctx).queryOne(sel, &row); err != nil {
		return false, err
	}
	return row.Count > 0, nil
}

func (r *pluginKVRepository) List(ctx context.Context, pluginID, prefix string) ([]string, error) {
	pred := And{Eq{"plugin_id": pluginID}, Expr(pluginKVNotExpired)}
	if prefix != "" {
		pred = append(pred, Expr("key LIKE ? ESCAPE '\\'", escapeLike(prefix)+"%"))
	}
	sel := Select("key").From(r.tableName).Where(pred).OrderBy("key")
	var rows []struct{ Key string }
	if err := r.withCtx(ctx).queryAll(sel, &rows); err != nil {
		return nil, err
	}
	keys := make([]string, len(rows))
	for i, row := range rows {
		keys[i] = row.Key
	}
	return keys, nil
}

func (r *pluginKVRepository) StorageUsed(ctx context.Context, pluginID string) (int64, error) {
	sel := Select("COALESCE(SUM(size), 0) as used").From(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Expr(pluginKVNotExpired),
	})
	var row struct{ Used int64 }
	if err := r.withCtx(ctx).queryOne(sel, &row); err != nil {
		return 0, err
	}
	return row.Used, nil
}

func (r *pluginKVRepository) ExistingSize(ctx context.Context, pluginID, key string) (int64, error) {
	sel := Select("COALESCE(size, 0) as size").From(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Eq{"key": key},
		Expr(pluginKVNotExpired),
	})
	var row struct{ Size int64 }
	err := r.withCtx(ctx).queryOne(sel, &row)
	if errors.Is(err, model.ErrNotFound) {
		return 0, nil
	}
	return row.Size, err
}

func (r *pluginKVRepository) CleanupExpired(ctx context.Context, pluginID string) (int64, error) {
	return r.withCtx(ctx).executeSQL(Delete(r.tableName).Where(And{
		Eq{"plugin_id": pluginID},
		Expr("expires_at IS NOT NULL AND expires_at < datetime('now')"),
	}))
}

func (r *pluginKVRepository) CountAll(ctx context.Context, pluginID string) (int64, error) {
	sel := Select("COUNT(*) as count").From(r.tableName).Where(Eq{"plugin_id": pluginID})
	var row struct{ Count int64 }
	if err := r.withCtx(ctx).queryOne(sel, &row); err != nil {
		return 0, err
	}
	return row.Count, nil
}

func (r *pluginKVRepository) ExpiresAt(ctx context.Context, pluginID, key string) (*string, error) {
	sel := Select("expires_at").From(r.tableName).Where(And{Eq{"plugin_id": pluginID}, Eq{"key": key}})
	var row struct{ ExpiresAt sql.NullString }
	err := r.withCtx(ctx).queryOne(sel, &row)
	if errors.Is(err, model.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !row.ExpiresAt.Valid {
		return nil, nil
	}
	v := row.ExpiresAt.String
	return &v, nil
}

func (r *pluginKVRepository) withCtx(ctx context.Context) *pluginKVRepository {
	clone := *r
	clone.ctx = ctx
	return &clone
}

func escapeLike(prefix string) string {
	escaped := strings.ReplaceAll(prefix, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, "%", `\%`)
	escaped = strings.ReplaceAll(escaped, "_", `\_`)
	return escaped
}
