package tests

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
)

type mockKVItem struct {
	value     []byte
	expiresAt *time.Time
}

type MockPluginKVRepo struct {
	mu   sync.Mutex
	data map[string]map[string]mockKVItem
}

func NewMockPluginKVRepo() *MockPluginKVRepo {
	return &MockPluginKVRepo{data: map[string]map[string]mockKVItem{}}
}

func (r *MockPluginKVRepo) ns(pluginID string) map[string]mockKVItem {
	if r.data[pluginID] == nil {
		r.data[pluginID] = map[string]mockKVItem{}
	}
	return r.data[pluginID]
}

func (r *MockPluginKVRepo) live(item mockKVItem) bool {
	return item.expiresAt == nil || item.expiresAt.After(time.Now())
}

func (r *MockPluginKVRepo) Get(_ context.Context, pluginID, key string) ([]byte, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.ns(pluginID)[key]
	if !ok || !r.live(item) {
		return nil, false, nil
	}
	return append([]byte(nil), item.value...), true, nil
}

func (r *MockPluginKVRepo) GetMany(ctx context.Context, pluginID string, keys []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, key := range keys {
		value, found, err := r.Get(ctx, pluginID, key)
		if err != nil {
			return nil, err
		}
		if found {
			out[key] = value
		}
	}
	return out, nil
}

func (r *MockPluginKVRepo) Put(_ context.Context, pluginID, key string, value []byte, ttlSeconds int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := mockKVItem{value: append([]byte(nil), value...)}
	if ttlSeconds > 0 {
		exp := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
		item.expiresAt = &exp
	}
	r.ns(pluginID)[key] = item
	return nil
}

func (r *MockPluginKVRepo) PutExpired(_ context.Context, pluginID, key string, value []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp := time.Now().Add(-time.Second)
	r.ns(pluginID)[key] = mockKVItem{value: append([]byte(nil), value...), expiresAt: &exp}
	return nil
}

func (r *MockPluginKVRepo) Delete(_ context.Context, pluginID, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ns(pluginID), key)
	return nil
}

func (r *MockPluginKVRepo) DeleteByPrefix(_ context.Context, pluginID, prefix string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for key := range r.ns(pluginID) {
		if strings.HasPrefix(key, prefix) {
			delete(r.data[pluginID], key)
			n++
		}
	}
	return n, nil
}

func (r *MockPluginKVRepo) Has(_ context.Context, pluginID, key string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.ns(pluginID)[key]
	return ok && r.live(item), nil
}

func (r *MockPluginKVRepo) List(_ context.Context, pluginID, prefix string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var keys []string
	for key, item := range r.ns(pluginID) {
		if !r.live(item) {
			continue
		}
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (r *MockPluginKVRepo) StorageUsed(_ context.Context, pluginID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var used int64
	for _, item := range r.ns(pluginID) {
		if r.live(item) {
			used += int64(len(item.value))
		}
	}
	return used, nil
}

func (r *MockPluginKVRepo) ExistingSize(ctx context.Context, pluginID, key string) (int64, error) {
	value, found, err := r.Get(ctx, pluginID, key)
	if err != nil || !found {
		return 0, err
	}
	return int64(len(value)), nil
}

func (r *MockPluginKVRepo) CleanupExpired(_ context.Context, pluginID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for key, item := range r.ns(pluginID) {
		if !r.live(item) {
			delete(r.data[pluginID], key)
			n++
		}
	}
	return n, nil
}

func (r *MockPluginKVRepo) CountAll(_ context.Context, pluginID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.ns(pluginID))), nil
}

func (r *MockPluginKVRepo) ExpiresAt(_ context.Context, pluginID, key string) (*string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.ns(pluginID)[key]
	if !ok || item.expiresAt == nil {
		return nil, nil
	}
	s := item.expiresAt.Format(time.RFC3339)
	return &s, nil
}

type mockTask struct {
	model.PluginTaskRecord
	UpdatedAt int64
}

type MockPluginTaskRepo struct {
	mu     sync.Mutex
	queues map[string]map[string]model.PluginQueueConfig
	tasks  map[string]*mockTask
}

func NewMockPluginTaskRepo() *MockPluginTaskRepo {
	return &MockPluginTaskRepo{
		queues: map[string]map[string]model.PluginQueueConfig{},
		tasks:  map[string]*mockTask{},
	}
}

func (r *MockPluginTaskRepo) taskKey(pluginID, taskID string) string {
	return pluginID + "\x00" + taskID
}

func (r *MockPluginTaskRepo) UpsertQueue(_ context.Context, pluginID string, cfg model.PluginQueueConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.queues[pluginID] == nil {
		r.queues[pluginID] = map[string]model.PluginQueueConfig{}
	}
	r.queues[pluginID][cfg.Name] = cfg
	return nil
}

func (r *MockPluginTaskRepo) ResetRunningToPending(_ context.Context, pluginID, queueName string, now int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if strings.HasPrefix(task.ID, "") && task.QueueName == queueName && task.Status == "running" && r.owns(pluginID, task) {
			task.Status = "pending"
			task.UpdatedAt = now
		}
	}
	return nil
}

func (r *MockPluginTaskRepo) owns(pluginID string, task *mockTask) bool {
	_, ok := r.tasks[r.taskKey(pluginID, task.ID)]
	return ok
}

func (r *MockPluginTaskRepo) ResetAllRunningToPending(_ context.Context, pluginID string, now int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, task := range r.tasks {
		if strings.HasPrefix(key, pluginID+"\x00") && task.Status == "running" {
			task.Status = "pending"
			task.UpdatedAt = now
		}
	}
	return nil
}

func (r *MockPluginTaskRepo) Enqueue(_ context.Context, pluginID, queueName, taskID string, payload []byte, maxRetries int32, now int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[r.taskKey(pluginID, taskID)] = &mockTask{
		PluginTaskRecord: model.PluginTaskRecord{
			ID:         taskID,
			QueueName:  queueName,
			Payload:    append([]byte(nil), payload...),
			Status:     "pending",
			Attempt:    0,
			MaxRetries: maxRetries,
			NextRunAt:  now,
		},
		UpdatedAt: now,
	}
	return nil
}

func (r *MockPluginTaskRepo) Get(_ context.Context, pluginID, taskID string) (*model.PluginTaskRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[r.taskKey(pluginID, taskID)]
	if !ok {
		return nil, sql.ErrNoRows
	}
	cp := task.PluginTaskRecord
	return &cp, nil
}

func (r *MockPluginTaskRepo) CancelPending(_ context.Context, pluginID, taskID string, now int64) (bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[r.taskKey(pluginID, taskID)]
	if !ok {
		return false, "", sql.ErrNoRows
	}
	if task.Status != "pending" {
		return false, task.Status, nil
	}
	task.Status = "cancelled"
	task.UpdatedAt = now
	return true, "cancelled", nil
}

func (r *MockPluginTaskRepo) ClearPending(_ context.Context, pluginID, queueName string, now int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	prefix := pluginID + "\x00"
	for key, task := range r.tasks {
		if strings.HasPrefix(key, prefix) && task.QueueName == queueName && task.Status == "pending" {
			task.Status = "cancelled"
			task.UpdatedAt = now
			n++
		}
	}
	return n, nil
}

func (r *MockPluginTaskRepo) Dequeue(_ context.Context, pluginID, queueName string, now int64) (*model.PluginTaskRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var best *mockTask
	prefix := pluginID + "\x00"
	for key, task := range r.tasks {
		if !strings.HasPrefix(key, prefix) || task.QueueName != queueName || task.Status != "pending" || task.NextRunAt > now {
			continue
		}
		if best == nil || task.NextRunAt < best.NextRunAt {
			best = task
		}
	}
	if best == nil {
		return nil, nil
	}
	best.Status = "running"
	best.Attempt++
	best.UpdatedAt = now
	cp := best.PluginTaskRecord
	return &cp, nil
}

func (r *MockPluginTaskRepo) Complete(_ context.Context, pluginID, taskID, message string, now int64) error {
	return r.setStatus(pluginID, taskID, "completed", message, now)
}

func (r *MockPluginTaskRepo) Fail(_ context.Context, pluginID, taskID, message string, now int64) error {
	return r.setStatus(pluginID, taskID, "failed", message, now)
}

func (r *MockPluginTaskRepo) setStatus(pluginID, taskID, status, message string, now int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[r.taskKey(pluginID, taskID)]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	task.Status = status
	task.Message = message
	task.UpdatedAt = now
	return nil
}

func (r *MockPluginTaskRepo) Reschedule(_ context.Context, pluginID, taskID string, nextRunAt, now int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[r.taskKey(pluginID, taskID)]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	task.Status = "pending"
	task.NextRunAt = nextRunAt
	task.UpdatedAt = now
	return nil
}

func (r *MockPluginTaskRepo) RevertToPending(_ context.Context, pluginID, taskID string, now int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[r.taskKey(pluginID, taskID)]
	if !ok || task.Status != "running" {
		return nil
	}
	task.Status = "pending"
	if task.Attempt > 0 {
		task.Attempt--
	}
	task.UpdatedAt = now
	return nil
}

func (r *MockPluginTaskRepo) CleanupTerminal(_ context.Context, pluginID, queueName string, retentionMs, now int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	prefix := pluginID + "\x00"
	for key, task := range r.tasks {
		if !strings.HasPrefix(key, prefix) || task.QueueName != queueName {
			continue
		}
		if task.Status != "completed" && task.Status != "failed" && task.Status != "cancelled" {
			continue
		}
		if task.UpdatedAt+retentionMs < now {
			delete(r.tasks, key)
			n++
		}
	}
	return n, nil
}

func (r *MockPluginTaskRepo) NextRunAt(_ context.Context, pluginID, taskID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[r.taskKey(pluginID, taskID)]
	if !ok {
		return 0, sql.ErrNoRows
	}
	return task.NextRunAt, nil
}
