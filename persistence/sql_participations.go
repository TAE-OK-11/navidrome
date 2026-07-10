package persistence

import (
	"fmt"
	"hash/maphash"
	"sync"

	. "github.com/Masterminds/squirrel"
	json "github.com/goccy/go-json"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/slice"
)

type participant struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	SubRole string `json:"subRole,omitempty"`
}

// flatParticipant represents a flattened participant structure for SQL processing
type flatParticipant struct {
	ArtistID string `json:"artist_id"`
	Role     string `json:"role"`
	SubRole  string `json:"sub_role,omitempty"`
}

const (
	participantCacheShardCount      = 16
	participantCacheEntriesPerShard = 16
	participantCacheMaxKeySize      = 16 << 10
)

type participantCacheEntry struct {
	source       string
	participants map[model.Role][]participant
}

type participantCacheShard struct {
	sync.RWMutex
	entries map[uint64]participantCacheEntry
}

var (
	participantCacheSeed = maphash.MakeSeed()
	participantCache     [participantCacheShardCount]participantCacheShard
)

func marshalParticipants(participants model.Participants) string {
	dbParticipants := make(map[model.Role][]participant)
	for role, artists := range participants {
		for _, artist := range artists {
			dbParticipants[role] = append(dbParticipants[role], participant{ID: artist.ID, SubRole: artist.SubRole, Name: artist.Name})
		}
	}
	res, _ := json.Marshal(dbParticipants)
	return string(res)
}

func unmarshalParticipants(data string) (model.Participants, error) {
	if len(data) > participantCacheMaxKeySize {
		dbParticipants, err := decodeParticipants(data)
		if err != nil {
			return nil, err
		}
		return participantsFromDB(dbParticipants), nil
	}

	key := maphash.String(participantCacheSeed, data)
	shard := &participantCache[int(key)&(participantCacheShardCount-1)]
	shard.RLock()
	entry, ok := shard.entries[key]
	shard.RUnlock()
	if !ok || entry.source != data {
		var err error
		entry.participants, err = decodeParticipants(data)
		if err != nil {
			return nil, err
		}
		entry.source = data
		shard.Lock()
		if shard.entries == nil {
			shard.entries = make(map[uint64]participantCacheEntry, participantCacheEntriesPerShard)
		} else if len(shard.entries) >= participantCacheEntriesPerShard {
			clear(shard.entries)
		}
		shard.entries[key] = entry
		shard.Unlock()
	}
	return participantsFromDB(entry.participants), nil
}

func decodeParticipants(data string) (map[model.Role][]participant, error) {
	var dbParticipants map[model.Role][]participant
	err := json.Unmarshal([]byte(data), &dbParticipants)
	if err != nil {
		return nil, fmt.Errorf("parsing participants: %w", err)
	}
	return dbParticipants, nil
}

func participantsFromDB(dbParticipants map[model.Role][]participant) model.Participants {
	participants := make(model.Participants, len(dbParticipants))
	for role, participantList := range dbParticipants {
		artists := make(model.ParticipantList, len(participantList))
		for i, p := range participantList {
			artists[i] = model.Participant{Artist: model.Artist{ID: p.ID, Name: p.Name}, SubRole: p.SubRole}
		}
		participants[role] = artists
	}
	return participants
}

func (r sqlRepository) updateParticipants(itemID string, participants model.Participants) error {
	// Delete all existing participant entries for this item.
	// This ensures stale role associations are removed when an artist's role changes
	// (e.g., an artist was both albumartist and composer, but is now only composer).
	sqd := Delete(r.tableName + "_artists").Where(Eq{r.tableName + "_id": itemID})
	_, err := r.executeSQL(sqd)
	if err != nil {
		return err
	}
	if len(participants) == 0 {
		return nil
	}

	var flatParticipants []flatParticipant
	for role, artists := range participants {
		for _, artist := range artists {
			flatParticipants = append(flatParticipants, flatParticipant{
				ArtistID: artist.ID,
				Role:     role.String(),
				SubRole:  artist.SubRole,
			})
		}
	}

	participantsJSON, err := json.Marshal(flatParticipants)
	if err != nil {
		return fmt.Errorf("marshaling participants: %w", err)
	}

	// Build the INSERT query using json_each and INNER JOIN to artist table
	// to automatically filter out non-existent artist IDs
	query := fmt.Sprintf(`
		INSERT INTO %[1]s_artists (%[1]s_id, artist_id, role, sub_role)
		SELECT ?, 
		       json_extract(value, '$.artist_id') as artist_id,
		       json_extract(value, '$.role') as role,
		       COALESCE(json_extract(value, '$.sub_role'), '') as sub_role
		-- Parse the flat JSON array: [{"artist_id": "id", "role": "role", "sub_role": "subRole"}]
		FROM json_each(?)                                        -- Iterate through each array element
		-- CRITICAL: Only insert records for artists that actually exist in the database
		JOIN artist ON artist.id = json_extract(value, '$.artist_id')  -- Filter out non-existent artist IDs via INNER JOIN
		-- Handle duplicate insertions gracefully (e.g., if called multiple times)
		ON CONFLICT (artist_id, %[1]s_id, role, sub_role) DO NOTHING   -- Ignore duplicates
	`, r.tableName)

	_, err = r.executeSQL(Expr(query, itemID, string(participantsJSON)))
	return err
}

func (r *sqlRepository) getParticipants(m *model.MediaFile) (model.Participants, error) {
	ar := NewArtistRepository(r.ctx, r.db)
	ids := m.Participants.AllIDs()
	artists, err := ar.GetAll(model.QueryOptions{Filters: Eq{"artist.id": ids}})
	if err != nil {
		return nil, fmt.Errorf("getting participants: %w", err)
	}
	artistMap := slice.ToMap(artists, func(a model.Artist) (string, model.Artist) {
		return a.ID, a
	})
	p := m.Participants
	for role, artistList := range p {
		for idx, artist := range artistList {
			if a, ok := artistMap[artist.ID]; ok {
				p[role][idx].Artist = a
			}
		}
	}
	return p, nil
}
