package model

import "context"

// PluginKVRepository is namespaced persistent key/value storage for plugins.
// Isolation is by plugin ID on the main DataStore, not a sidecar SQLite file.
type PluginKVRepository interface {
	Get(ctx context.Context, pluginID, key string) (value []byte, found bool, err error)
	GetMany(ctx context.Context, pluginID string, keys []string) (map[string][]byte, error)
	Put(ctx context.Context, pluginID, key string, value []byte, ttlSeconds int64) error
	PutExpired(ctx context.Context, pluginID, key string, value []byte) error
	Delete(ctx context.Context, pluginID, key string) error
	DeleteByPrefix(ctx context.Context, pluginID, prefix string) (int64, error)
	Has(ctx context.Context, pluginID, key string) (bool, error)
	List(ctx context.Context, pluginID, prefix string) ([]string, error)
	StorageUsed(ctx context.Context, pluginID string) (int64, error)
	ExistingSize(ctx context.Context, pluginID, key string) (int64, error)
	CleanupExpired(ctx context.Context, pluginID string) (int64, error)
	CountAll(ctx context.Context, pluginID string) (int64, error)
	ExpiresAt(ctx context.Context, pluginID, key string) (expiresAt *string, err error)
}

// PluginQueueConfig is the persisted worker configuration for one plugin queue.
type PluginQueueConfig struct {
	Name        string
	Concurrency int32
	MaxRetries  int32
	BackoffMs   int64
	DelayMs     int64
	RetentionMs int64
}

// PluginTaskRecord is one row in the shared plugin task table.
type PluginTaskRecord struct {
	ID         string
	QueueName  string
	Payload    []byte
	Status     string
	Message    string
	Attempt    int32
	MaxRetries int32
	NextRunAt  int64
}

// PluginTaskRepository is namespaced task-queue storage for plugins.
type PluginTaskRepository interface {
	UpsertQueue(ctx context.Context, pluginID string, cfg PluginQueueConfig) error
	ResetRunningToPending(ctx context.Context, pluginID, queueName string, now int64) error
	ResetAllRunningToPending(ctx context.Context, pluginID string, now int64) error
	Enqueue(ctx context.Context, pluginID, queueName, taskID string, payload []byte, maxRetries int32, now int64) error
	Get(ctx context.Context, pluginID, taskID string) (*PluginTaskRecord, error)
	CancelPending(ctx context.Context, pluginID, taskID string, now int64) (cancelled bool, status string, err error)
	ClearPending(ctx context.Context, pluginID, queueName string, now int64) (int64, error)
	Dequeue(ctx context.Context, pluginID, queueName string, now int64) (*PluginTaskRecord, error)
	Complete(ctx context.Context, pluginID, taskID, message string, now int64) error
	Fail(ctx context.Context, pluginID, taskID, message string, now int64) error
	Reschedule(ctx context.Context, pluginID, taskID string, nextRunAt, now int64) error
	RevertToPending(ctx context.Context, pluginID, taskID string, now int64) error
	CleanupTerminal(ctx context.Context, pluginID, queueName string, retentionMs, now int64) (int64, error)
	NextRunAt(ctx context.Context, pluginID, taskID string) (int64, error)
}
