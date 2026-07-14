package hotcache

import (
	"context"
	"errors"
	"os"

	"github.com/navidrome/navidrome/model"
)

var (
	errHotCacheDisabled              = errors.New("hot cache is disabled")
	disabledResolverInstance Manager = disabledManager{}
)

// disabledManager keeps the administrator API stable without allocating cache
// maps, queues, workers, or observation wrappers. The streaming package detects
// this resolver once during construction and opens source files directly.
type disabledManager struct{}

func (disabledManager) Open(_ context.Context, mf *model.MediaFile) (File, error) {
	return os.Open(mf.AbsolutePath())
}

func (disabledManager) CacheEnabled() bool { return false }
func (disabledManager) Stats() Stats       { return Stats{} }
func (disabledManager) Status() StatusSnapshot {
	return StatusSnapshot{Health: "disabled"}
}
func (disabledManager) Sessions() []SessionSnapshot { return []SessionSnapshot{} }
func (disabledManager) Entries(EntryQuery) EntryPage {
	return EntryPage{Items: []EntrySnapshot{}}
}
func (disabledManager) Queue() []QueueItemSnapshot                      { return []QueueItemSnapshot{} }
func (disabledManager) CurrentPromotion() *PromotionSnapshot            { return nil }
func (disabledManager) Events(uint64, int) []Event                      { return []Event{} }
func (disabledManager) Errors(uint64, int) []Event                      { return []Event{} }
func (disabledManager) Formats() []FormatSnapshot                       { return []FormatSnapshot{} }
func (disabledManager) Promote(context.Context, *model.MediaFile) error { return errHotCacheDisabled }
func (disabledManager) Retry(context.Context, *model.MediaFile) error   { return errHotCacheDisabled }
func (disabledManager) Remove(string) error                             { return errHotCacheDisabled }
func (disabledManager) Cancel(string) error                             { return errHotCacheDisabled }
func (disabledManager) Pause()                                          {}
func (disabledManager) Resume()                                         {}
func (disabledManager) Verify(context.Context) VerificationResult       { return VerificationResult{} }
func (disabledManager) Cleanup(context.Context) CleanupResult           { return CleanupResult{} }
func (disabledManager) Purge(context.Context) PurgeResult               { return PurgeResult{} }
func (disabledManager) ResetEvents()                                    {}
func (disabledManager) ResetStats()                                     {}
func (disabledManager) Shutdown(context.Context) error                  { return nil }

func (disabledManager) MediaStates(mediaIDs []string) map[string]string {
	states := make(map[string]string, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		if mediaID != "" {
			states[mediaID] = "disabled"
		}
	}
	return states
}
