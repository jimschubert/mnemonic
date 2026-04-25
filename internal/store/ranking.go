package store

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"
)

// SortByWeightedScore sorts entries by weighted score descending, then by ID.
func SortByWeightedScore(entries []Entry) {
	slices.SortStableFunc(entries, func(a, b Entry) int {
		sa, sb := WeightedScore(a), WeightedScore(b)
		// use epsilon to compare because floats are annoying
		if math.Abs(sa-sb) < 1e-10 {
			// basically, zero
			return strings.Compare(a.ID, b.ID)
		}
		// sort descending (higher scores first)
		return cmp.Compare(sb, sa)
	})
}

// WeightedScore adds a recency bias to the entry's score. Recent hits increase the score, while old entries decay over time.
// The half-life is ~14 days (ln(2) / 0.05). LastHit is used as the recency reference; Created is the fallback for
// entries that have never been queried or reinforced.
func WeightedScore(entry Entry) float64 {
	ref := entry.LastHit
	if ref.IsZero() {
		ref = entry.Created
	}
	days := time.Since(ref).Hours() / 24
	decay := math.Exp(-0.05 * days)
	return entry.Score * decay
}
