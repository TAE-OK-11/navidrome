package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	. "github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/pocketbase/dbx"
)

type pluginTaskRepository struct {
	sqlRepository
}

func NewPluginTaskRepository(ctx context.Context, db dbx.Builder) model.PluginTaskRepository {
	r := &pluginTaskRepository{}
	r.ctx = ctx
	r.db = db
	r.tableName = "plugin_task"
	return r
}

func (r *pluginTaskRepository) UpsertQueue(ctx context.Context, pluginID string, cfg model.PluginQueueConfig) error {
	_, err := r.db.NewQuery(`
		INSERT INTO plugin_queue (plugin_id, name, concurrency, max_retries, backoff_ms, delay_ms, retention_ms)
		VALUES ({:plugin}, {:name}, {:concurrency}, {:retries}, {:backoff}, {:delay}, {:retention})
		ON CONFLICT(plugin_id, name) DO UPDATE SET
			concurrency = excluded.concurrency,
			max_retries = excluded.max_retries,
			backoff_ms = excluded.backoff_ms,
			delay_ms = excluded.delay_ms,
			retention_ms = excluded.retention_ms
	`).Bind(dbx.Params{
		"plugin":      pluginID,
		"name":        cfg.Name,
		"concurrency": cfg.Concurrency,
		"retries":     cfg.MaxRetries,
		"backoff":     cfg.BackoffMs,
		"delay":       cfg.DelayMs,
		"retention":   cfg.RetentionMs,
	}).WithContext(ctx).Execute()
	if err != nil {
		return fmt.Errorf("creating plugin queue: %w", err)
	}
	return nil
}

func (r *pluginTaskRepository) ResetRunningToPending(ctx context.Context, pluginID, queueName string, now int64) error {
	_, err := r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskPending).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"queue_name": queueName}, Eq{"status": model.PluginTaskRunning}}))
	return err
}

func (r *pluginTaskRepository) ResetAllRunningToPending(ctx context.Context, pluginID string, now int64) error {
	_, err := r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskPending).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"status": model.PluginTaskRunning}}))
	return err
}

func (r *pluginTaskRepository) Enqueue(ctx context.Context, pluginID, queueName, taskID string, payload []byte, maxRetries int32, now int64) error {
	_, err := r.withCtx(ctx).executeSQL(Insert("plugin_task").Columns(
		"id", "plugin_id", "queue_name", "payload", "status", "attempt", "max_retries", "next_run_at", "created_at", "updated_at",
	).Values(taskID, pluginID, queueName, payload, model.PluginTaskPending, 0, maxRetries, now, now, now))
	if err != nil {
		return fmt.Errorf("enqueuing plugin task: %w", err)
	}
	return nil
}

func (r *pluginTaskRepository) Get(ctx context.Context, pluginID, taskID string) (*model.PluginTaskRecord, error) {
	sel := Select("id", "queue_name", "payload", "status", "message", "attempt", "max_retries", "next_run_at").
		From("plugin_task").Where(And{Eq{"plugin_id": pluginID}, Eq{"id": taskID}})
	var row model.PluginTaskRecord
	err := r.withCtx(ctx).queryOne(sel, &row)
	if errors.Is(err, model.ErrNotFound) {
		return nil, sql.ErrNoRows
	}
	return &row, err
}

func (r *pluginTaskRepository) CancelPending(ctx context.Context, pluginID, taskID string, now int64) (bool, string, error) {
	count, err := r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskCancelled).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"id": taskID}, Eq{"status": model.PluginTaskPending}}))
	if err != nil {
		return false, "", err
	}
	if count > 0 {
		return true, model.PluginTaskCancelled, nil
	}
	rec, err := r.Get(ctx, pluginID, taskID)
	if err != nil {
		return false, "", err
	}
	return false, rec.Status, nil
}

func (r *pluginTaskRepository) ClearPending(ctx context.Context, pluginID, queueName string, now int64) (int64, error) {
	return r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskCancelled).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"queue_name": queueName}, Eq{"status": model.PluginTaskPending}}))
}

func (r *pluginTaskRepository) Dequeue(ctx context.Context, pluginID, queueName string, now int64) (*model.PluginTaskRecord, error) {
	var rec model.PluginTaskRecord
	err := r.db.NewQuery(`
		UPDATE plugin_task SET status = {:running}, attempt = attempt + 1, updated_at = {:now}
		WHERE id = (
			SELECT id FROM plugin_task
			WHERE plugin_id = {:plugin} AND queue_name = {:queue} AND status = {:pending} AND next_run_at <= {:now}
			ORDER BY next_run_at, created_at LIMIT 1
		)
		RETURNING id, queue_name, payload, status, message, attempt, max_retries, next_run_at
	`).Bind(dbx.Params{
		"running": model.PluginTaskRunning,
		"now":     now,
		"plugin":  pluginID,
		"queue":   queueName,
		"pending": model.PluginTaskPending,
	}).WithContext(ctx).One(&rec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *pluginTaskRepository) Complete(ctx context.Context, pluginID, taskID, message string, now int64) error {
	_, err := r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskCompleted).
		Set("message", message).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"id": taskID}}))
	return err
}

func (r *pluginTaskRepository) Fail(ctx context.Context, pluginID, taskID, message string, now int64) error {
	_, err := r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskFailed).
		Set("message", message).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"id": taskID}}))
	return err
}

func (r *pluginTaskRepository) Reschedule(ctx context.Context, pluginID, taskID string, nextRunAt, now int64) error {
	_, err := r.withCtx(ctx).executeSQL(Update("plugin_task").
		Set("status", model.PluginTaskPending).
		Set("next_run_at", nextRunAt).
		Set("updated_at", now).
		Where(And{Eq{"plugin_id": pluginID}, Eq{"id": taskID}}))
	return err
}

func (r *pluginTaskRepository) RevertToPending(ctx context.Context, pluginID, taskID string, now int64) error {
	_, err := r.db.NewQuery(`
		UPDATE plugin_task SET status = {:pending}, attempt = MAX(attempt - 1, 0), updated_at = {:now}
		WHERE plugin_id = {:plugin} AND id = {:id} AND status = {:running}
	`).Bind(dbx.Params{
		"pending": model.PluginTaskPending,
		"now":     now,
		"plugin":  pluginID,
		"id":      taskID,
		"running": model.PluginTaskRunning,
	}).WithContext(ctx).Execute()
	return err
}

func (r *pluginTaskRepository) CleanupTerminal(ctx context.Context, pluginID, queueName string, retentionMs, now int64) (int64, error) {
	res, err := r.db.NewQuery(`
		DELETE FROM plugin_task
		WHERE plugin_id = {:plugin} AND queue_name = {:queue}
		  AND status IN ({:completed}, {:failed}, {:cancelled})
		  AND updated_at + {:retention} < {:now}
	`).Bind(dbx.Params{
		"plugin":    pluginID,
		"queue":     queueName,
		"completed": model.PluginTaskCompleted,
		"failed":    model.PluginTaskFailed,
		"cancelled": model.PluginTaskCancelled,
		"retention": retentionMs,
		"now":       now,
	}).WithContext(ctx).Execute()
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *pluginTaskRepository) NextRunAt(ctx context.Context, pluginID, taskID string) (int64, error) {
	sel := Select("next_run_at").From("plugin_task").Where(And{Eq{"plugin_id": pluginID}, Eq{"id": taskID}})
	var row struct{ NextRunAt int64 }
	err := r.withCtx(ctx).queryOne(sel, &row)
	return row.NextRunAt, err
}

func (r *pluginTaskRepository) withCtx(ctx context.Context) *pluginTaskRepository {
	clone := *r
	clone.ctx = ctx
	return &clone
}
