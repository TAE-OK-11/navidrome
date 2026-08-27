package model

import (
	"slices"

	"github.com/navidrome/navidrome/utils/gg"
)

// NormalizeCueEnds resolves missing cue end times within a single ordered cue
// group: each end is filled from the next cue's start, then from fallbackEnd,
// and is clamped so it never precedes the cue's own start nor overruns the next
// cue. End times are all-or-none — if any cue still lacks an end afterwards, all
// ends in the group are cleared. The input slice is never mutated.
//
// Exported because the Subsonic enhanced-lyrics serializer resolves cue ends
// per agent group while building the response.
func NormalizeCueEnds(cues []Cue, fallbackEnd *int64) []Cue {
	if len(cues) == 0 {
		return cues
	}

	out := slices.Clone(cues)
	for i := range out {
		end := out[i].End
		if end == nil {
			if i+1 < len(out) && out[i+1].Start != nil {
				end = out[i+1].Start
			} else {
				end = fallbackEnd
			}
		}
		if end != nil && i+1 < len(out) && out[i+1].Start != nil && *end > *out[i+1].Start {
			end = out[i+1].Start
		}
		if end != nil && out[i].Start != nil && *end < *out[i].Start {
			end = out[i].Start
		}
		out[i].End = gg.Clone(end)
	}

	for i := range out {
		if out[i].End == nil {
			for j := range out {
				out[j].End = nil
			}
			break
		}
	}
	return out
}
