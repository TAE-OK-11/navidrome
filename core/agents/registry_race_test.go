package agents

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

type raceAgent struct{}

func (raceAgent) AgentName() string { return "race" }

func TestRegistryConcurrentAccess(t *testing.T) {
	ds := &tests.MockDataStore{}
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			Register(fmt.Sprintf("concurrent-%d", i), func(model.DataStore) Interface {
				return raceAgent{}
			})
		}(i)
		go func() {
			defer wg.Done()
			ag := createAgents(ds, nil)
			_, _ = ag.GetArtistMBID(context.Background(), "id", "name")
		}()
	}
	wg.Wait()
}
