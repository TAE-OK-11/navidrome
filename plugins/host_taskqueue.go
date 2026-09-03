package plugins

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/id"
	"github.com/navidrome/navidrome/plugins/capabilities"
	"github.com/navidrome/navidrome/plugins/host"
	"golang.org/x/time/rate"
)

const (
	defaultConcurrency  int32 = 1
	defaultBackoffMs    int64 = 1000
	defaultRetentionMs  int64 = 3_600_000   // 1 hour
	minRetentionMs      int64 = 60_000      // 1 minute
	maxRetentionMs      int64 = 604_800_000 // 1 week
	maxQueueNameLength        = 128
	maxPayloadSize            = 1 * 1024 * 1024 // 1MB
	maxBackoffMs        int64 = 3_600_000       // 1 hour
	taskCleanupInterval       = 5 * time.Minute
	pollInterval              = 5 * time.Second
	shutdownTimeout           = 10 * time.Second
)

// CapabilityTaskWorker indicates the plugin can receive task execution callbacks.
const CapabilityTaskWorker Capability = "TaskWorker"

const FuncTaskWorkerCallback = "nd_task_execute"

func init() {
	registerCapability(CapabilityTaskWorker, FuncTaskWorkerCallback)
}

type queueState struct {
	config  host.QueueConfig
	signal  chan struct{}
	limiter *rate.Limiter
}

// notifyWorkers sends a non-blocking signal to wake up queue workers.
func (qs *queueState) notifyWorkers() {
	select {
	case qs.signal <- struct{}{}:
	default:
	}
}

// taskQueueServiceImpl implements host.TaskQueueService with SQLite persistence
// and background worker goroutines for task execution.
type taskQueueServiceImpl struct {
	pluginName     string
	manager        *Manager
	maxConcurrency int32
	store          model.PluginTaskRepository
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
	queues         map[string]*queueState
	closed         atomic.Bool

	// For testing: override how callbacks are invoked
	invokeCallbackFn func(ctx context.Context, queueName, taskID string, payload []byte, attempt int32) (string, error)
}

func newTaskQueueService(ctx context.Context, ds model.DataStore, pluginName string, manager *Manager, maxConcurrency int32) (*taskQueueServiceImpl, error) {
	if ds == nil {
		return nil, fmt.Errorf("plugin datastore is required")
	}
	store := ds.PluginTask(ctx)
	if store == nil {
		return nil, fmt.Errorf("plugin task store unavailable")
	}

	ctx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is stored in struct and called in Close()

	s := &taskQueueServiceImpl{
		pluginName:     pluginName,
		manager:        manager,
		maxConcurrency: maxConcurrency,
		store:          store,
		ctx:            ctx,
		cancel:         cancel,
		queues:         make(map[string]*queueState),
	}
	s.invokeCallbackFn = s.defaultInvokeCallback

	s.wg.Go(s.cleanupLoop)

	log.Debug("Initialized plugin taskqueue", "plugin", pluginName, "maxConcurrency", maxConcurrency)
	return s, nil
}

// applyConfigDefaults fills zero-value config fields with sensible defaults
// and clamps values to valid ranges, logging warnings for clamped values.
func (s *taskQueueServiceImpl) applyConfigDefaults(ctx context.Context, name string, config *host.QueueConfig) {
	if config.Concurrency <= 0 {
		config.Concurrency = defaultConcurrency
	}
	if config.BackoffMs <= 0 {
		config.BackoffMs = defaultBackoffMs
	}
	if config.RetentionMs <= 0 {
		config.RetentionMs = defaultRetentionMs
	}

	if config.RetentionMs < minRetentionMs {
		log.Warn(ctx, "TaskQueue retention clamped to minimum", "plugin", s.pluginName, "queue", name,
			"requested", config.RetentionMs, "min", minRetentionMs)
		config.RetentionMs = minRetentionMs
	}
	if config.RetentionMs > maxRetentionMs {
		log.Warn(ctx, "TaskQueue retention clamped to maximum", "plugin", s.pluginName, "queue", name,
			"requested", config.RetentionMs, "max", maxRetentionMs)
		config.RetentionMs = maxRetentionMs
	}
}

// clampConcurrency reduces config.Concurrency if it exceeds the remaining budget.
// Returns an error when the concurrency budget is fully exhausted.
// Must be called with s.mu held.
func (s *taskQueueServiceImpl) clampConcurrency(ctx context.Context, name string, config *host.QueueConfig) error {
	var allocated int32
	for _, qs := range s.queues {
		allocated += qs.config.Concurrency
	}
	available := s.maxConcurrency - allocated
	if available <= 0 {
		log.Warn(ctx, "TaskQueue concurrency budget exhausted", "plugin", s.pluginName, "queue", name,
			"allocated", allocated, "maxConcurrency", s.maxConcurrency)
		return fmt.Errorf("concurrency budget exhausted (%d/%d allocated)", allocated, s.maxConcurrency)
	}
	if config.Concurrency > available {
		log.Warn(ctx, "TaskQueue concurrency clamped", "plugin", s.pluginName, "queue", name,
			"requested", config.Concurrency, "available", available, "maxConcurrency", s.maxConcurrency)
		config.Concurrency = available
	}
	return nil
}

func (s *taskQueueServiceImpl) CreateQueue(ctx context.Context, name string, config host.QueueConfig) error {
	if s.closed.Load() {
		return fmt.Errorf("taskqueue is closed")
	}
	if len(name) == 0 {
		return fmt.Errorf("queue name cannot be empty")
	}
	if len(name) > maxQueueNameLength {
		return fmt.Errorf("queue name exceeds maximum length of %d bytes", maxQueueNameLength)
	}

	s.applyConfigDefaults(ctx, name, &config)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.clampConcurrency(ctx, name, &config); err != nil {
		return err
	}

	if _, exists := s.queues[name]; exists {
		return fmt.Errorf("queue %q already exists", name)
	}

	err := s.store.UpsertQueue(ctx, s.pluginName, model.PluginQueueConfig{
		Name:        name,
		Concurrency: config.Concurrency,
		MaxRetries:  config.MaxRetries,
		BackoffMs:   config.BackoffMs,
		DelayMs:     config.DelayMs,
		RetentionMs: config.RetentionMs,
	})
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	if err := s.store.ResetRunningToPending(ctx, s.pluginName, name, now); err != nil {
		return fmt.Errorf("resetting stale tasks: %w", err)
	}

	qs := &queueState{
		config: config,
		signal: make(chan struct{}, 1),
	}
	if config.DelayMs > 0 {
		qs.limiter = rate.NewLimiter(rate.Every(time.Duration(config.DelayMs)*time.Millisecond), 1)
	}
	s.queues[name] = qs

	for i := int32(0); i < config.Concurrency; i++ {
		s.wg.Go(func() { s.worker(name, qs) })
	}

	log.Debug(ctx, "Created task queue", "plugin", s.pluginName, "queue", name,
		"concurrency", config.Concurrency, "maxRetries", config.MaxRetries,
		"backoffMs", config.BackoffMs, "delayMs", config.DelayMs, "retentionMs", config.RetentionMs)
	return nil
}

func (s *taskQueueServiceImpl) Enqueue(ctx context.Context, queueName string, payload []byte) (string, error) {
	if s.closed.Load() {
		return "", fmt.Errorf("taskqueue is closed")
	}
	s.mu.Lock()
	qs, exists := s.queues[queueName]
	s.mu.Unlock()

	if !exists {
		return "", fmt.Errorf("queue %q does not exist", queueName)
	}
	if len(payload) > maxPayloadSize {
		return "", fmt.Errorf("payload size %d exceeds maximum of %d bytes", len(payload), maxPayloadSize)
	}

	taskID := id.NewRandom()
	now := time.Now().UnixMilli()
	if err := s.store.Enqueue(ctx, s.pluginName, queueName, taskID, payload, qs.config.MaxRetries, now); err != nil {
		return "", err
	}

	qs.notifyWorkers()
	log.Trace(ctx, "Enqueued task", "plugin", s.pluginName, "queue", queueName, "taskID", taskID)
	return taskID, nil
}

func (s *taskQueueServiceImpl) Get(ctx context.Context, taskID string) (*host.TaskInfo, error) {
	if s.closed.Load() {
		return nil, fmt.Errorf("taskqueue is closed")
	}
	rec, err := s.store.Get(ctx, s.pluginName, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task %q not found", taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("getting task info: %w", err)
	}
	return &host.TaskInfo{Status: rec.Status, Message: rec.Message, Attempt: rec.Attempt}, nil
}

func (s *taskQueueServiceImpl) Cancel(ctx context.Context, taskID string) error {
	if s.closed.Load() {
		return fmt.Errorf("taskqueue is closed")
	}
	now := time.Now().UnixMilli()
	cancelled, status, err := s.store.CancelPending(ctx, s.pluginName, taskID, now)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("task %q not found", taskID)
	}
	if err != nil {
		return fmt.Errorf("cancelling task: %w", err)
	}
	if !cancelled {
		return fmt.Errorf("task %q cannot be cancelled (status: %s)", taskID, status)
	}
	log.Trace(ctx, "Cancelled task", "plugin", s.pluginName, "taskID", taskID)
	return nil
}

// ClearQueue removes all pending tasks from the named queue.
// Running tasks are not affected. Returns the number of tasks removed.
func (s *taskQueueServiceImpl) ClearQueue(ctx context.Context, queueName string) (int64, error) {
	s.mu.Lock()
	_, exists := s.queues[queueName]
	s.mu.Unlock()

	if !exists {
		return 0, fmt.Errorf("queue %q does not exist", queueName)
	}

	now := time.Now().UnixMilli()
	cleared, err := s.store.ClearPending(ctx, s.pluginName, queueName, now)
	if err != nil {
		return 0, fmt.Errorf("clearing queue: %w", err)
	}

	if cleared > 0 {
		log.Debug(ctx, "Cleared pending tasks from queue", "plugin", s.pluginName, "queue", queueName, "cleared", cleared)
	}
	return cleared, nil
}

// worker is the main loop for a single worker goroutine.
func (s *taskQueueServiceImpl) worker(queueName string, qs *queueState) {
	// Process any existing pending tasks immediately on startup
	s.drainQueue(queueName, qs)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-qs.signal:
			s.drainQueue(queueName, qs)
		case <-ticker.C:
			s.drainQueue(queueName, qs)
		}
	}
}

func (s *taskQueueServiceImpl) drainQueue(queueName string, qs *queueState) {
	for s.ctx.Err() == nil && s.processTask(queueName, qs) {
	}
}

// processTask dequeues and processes a single task. Returns true if a task was processed.
func (s *taskQueueServiceImpl) processTask(queueName string, qs *queueState) bool {
	now := time.Now().UnixMilli()
	rec, err := s.store.Dequeue(s.ctx, s.pluginName, queueName, now)
	if err != nil {
		log.Error(s.ctx, "Failed to dequeue task", "plugin", s.pluginName, "queue", queueName, err)
		return false
	}
	if rec == nil {
		return false
	}
	taskID, payload, attempt, maxRetries := rec.ID, rec.Payload, rec.Attempt, rec.MaxRetries

	// Enforce delay between task dispatches using a rate limiter.
	// This is done after dequeue so that empty polls don't consume rate tokens.
	if qs.limiter != nil {
		if err := qs.limiter.Wait(s.ctx); err != nil {
			// Context cancelled during wait — revert task to pending for recovery
			s.revertTaskToPending(taskID)
			return false
		}
	}

	// Invoke callback
	log.Debug(s.ctx, "Executing task", "plugin", s.pluginName, "queue", queueName, "taskID", taskID, "attempt", attempt)
	message, callbackErr := s.invokeCallbackFn(s.ctx, queueName, taskID, payload, attempt)

	// If context was cancelled (shutdown), revert task to pending for recovery
	if s.ctx.Err() != nil {
		s.revertTaskToPending(taskID)
		return false
	}

	if callbackErr == nil {
		s.completeTask(queueName, taskID, message)
	} else {
		s.handleTaskFailure(queueName, taskID, attempt, maxRetries, qs, callbackErr, message)
	}
	return true
}

func (s *taskQueueServiceImpl) completeTask(queueName, taskID, message string) {
	now := time.Now().UnixMilli()
	if err := s.store.Complete(s.ctx, s.pluginName, taskID, message, now); err != nil {
		log.Error(s.ctx, "Failed to mark task as completed", "plugin", s.pluginName, "taskID", taskID, err)
	}
	log.Debug(s.ctx, "Task completed", "plugin", s.pluginName, "queue", queueName, "taskID", taskID)
}

func (s *taskQueueServiceImpl) handleTaskFailure(queueName, taskID string, attempt, maxRetries int32, qs *queueState, callbackErr error, message string) {
	log.Warn(s.ctx, "Task execution failed", "plugin", s.pluginName, "queue", queueName,
		"taskID", taskID, "attempt", attempt, "maxRetries", maxRetries, "err", callbackErr)

	// Use error message as fallback if no message was provided
	if message == "" {
		message = callbackErr.Error()
	}

	now := time.Now().UnixMilli()
	if attempt > maxRetries {
		if err := s.store.Fail(s.ctx, s.pluginName, taskID, message, now); err != nil {
			log.Error(s.ctx, "Failed to mark task as failed", "plugin", s.pluginName, "taskID", taskID, err)
		}
		log.Warn(s.ctx, "Task failed after all retries", "plugin", s.pluginName, "queue", queueName, "taskID", taskID)
		return
	}

	// Exponential backoff: backoffMs * 2^(attempt-1)
	backoff := qs.config.BackoffMs << (attempt - 1)
	if backoff <= 0 || backoff > maxBackoffMs {
		backoff = maxBackoffMs
	}
	nextRunAt := now + backoff
	if err := s.store.Reschedule(s.ctx, s.pluginName, taskID, nextRunAt, now); err != nil {
		log.Error(s.ctx, "Failed to reschedule task for retry", "plugin", s.pluginName, "taskID", taskID, err)
	}

	// Wake worker after backoff expires
	time.AfterFunc(time.Duration(backoff)*time.Millisecond, func() {
		qs.notifyWorkers()
	})
}

// revertTaskToPending puts a running task back to pending status and decrements the attempt
// counter (used during shutdown to ensure the interrupted attempt doesn't count).
func (s *taskQueueServiceImpl) revertTaskToPending(taskID string) {
	now := time.Now().UnixMilli()
	if err := s.store.RevertToPending(context.Background(), s.pluginName, taskID, now); err != nil {
		log.Error("Failed to revert task to pending", "plugin", s.pluginName, "taskID", taskID, err)
	}
}

// defaultInvokeCallback calls the plugin's nd_task_execute function.
func (s *taskQueueServiceImpl) defaultInvokeCallback(ctx context.Context, queueName, taskID string, payload []byte, attempt int32) (string, error) {
	s.manager.mu.RLock()
	p, ok := s.manager.plugins[s.pluginName]
	s.manager.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("plugin %s not loaded", s.pluginName)
	}

	input := capabilities.TaskExecuteRequest{
		QueueName: queueName,
		TaskID:    taskID,
		Payload:   payload,
		Attempt:   attempt,
	}

	message, err := callPluginFunction[capabilities.TaskExecuteRequest, string](ctx, p, FuncTaskWorkerCallback, input)
	if err != nil {
		return "", err
	}
	return message, nil
}

// cleanupLoop periodically removes terminal tasks past their retention period.
func (s *taskQueueServiceImpl) cleanupLoop() {
	ticker := time.NewTicker(taskCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runCleanup()
		}
	}
}

// runCleanup deletes terminal tasks past their retention period.
func (s *taskQueueServiceImpl) runCleanup() {
	s.mu.Lock()
	queues := make(map[string]*queueState, len(s.queues))
	maps.Copy(queues, s.queues)
	s.mu.Unlock()

	now := time.Now().UnixMilli()
	for name, qs := range queues {
		deleted, err := s.store.CleanupTerminal(s.ctx, s.pluginName, name, qs.config.RetentionMs, now)
		if err != nil {
			log.Error(s.ctx, "Failed to cleanup tasks", "plugin", s.pluginName, "queue", name, err)
			continue
		}
		if deleted > 0 {
			log.Debug(s.ctx, "Cleaned up terminal tasks", "plugin", s.pluginName, "queue", name, "deleted", deleted)
		}
	}
}

// Close shuts down the task queue service, stopping all workers and closing the database.
func (s *taskQueueServiceImpl) Close() error {
	s.closed.Store(true)
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		log.Warn("TaskQueue shutdown timed out", "plugin", s.pluginName)
	}

	now := time.Now().UnixMilli()
	if err := s.store.ResetAllRunningToPending(context.Background(), s.pluginName, now); err != nil {
		log.Error("Failed to reset running tasks on shutdown", "plugin", s.pluginName, err)
	}
	log.Debug("Closing plugin taskqueue", "plugin", s.pluginName)
	return nil
}

// Compile-time verification
var _ host.TaskService = (*taskQueueServiceImpl)(nil)
var _ io.Closer = (*taskQueueServiceImpl)(nil)
