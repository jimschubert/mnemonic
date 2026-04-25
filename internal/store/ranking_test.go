package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestWeightedScore_HigherBaseScoreWins(t *testing.T) {
	t.Parallel()

	now := time.Now()
	high := Entry{
		Score:   10.0,
		Created: now,
	}
	low := Entry{
		Score:   1.0,
		Created: now,
	}

	assert.True(t, WeightedScore(high) > WeightedScore(low))
}

func TestWeightedScore_RecentHitBeatsOldHit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	recent := Entry{
		Score:   1.0,
		LastHit: now,
	}
	old := Entry{
		Score:   1.0,
		LastHit: now.Add(-365 * 24 * time.Hour),
	}

	assert.True(t, WeightedScore(recent) > WeightedScore(old))
}

func TestWeightedScore_UsesLastHitOverCreated(t *testing.T) {
	t.Parallel()

	now := time.Now()
	veryOld := now.Add(-730 * 24 * time.Hour)

	entry := Entry{
		Score:   1.0,
		Created: veryOld,
		LastHit: now,
	}

	assert.True(t, WeightedScore(entry) > 0.99)
}

func TestWeightedScore_FallsBackToCreatedWhenNoLastHit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entry := Entry{
		Score:   1.0,
		Created: now,
		LastHit: time.Time{},
	}

	assert.True(t, WeightedScore(entry) > 0.99)
}

func TestWeightedScore_DecaysOverTime(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name     string
		duration time.Duration
		minScore float64
		maxScore float64
	}{
		{
			name:     "recent (1 hour ago)",
			duration: 1 * time.Hour,
			minScore: 0.99,
			maxScore: 1.0,
		},
		{
			name:     "one day ago",
			duration: 24 * time.Hour,
			minScore: 0.95,
			maxScore: 0.99,
		},
		{
			name:     "one week ago",
			duration: 7 * 24 * time.Hour,
			minScore: 0.70,
			maxScore: 0.85,
		},
		{
			name:     "14 days ago (half-life)",
			duration: 14 * 24 * time.Hour,
			minScore: 0.49,
			maxScore: 0.51,
		},
		{
			name:     "365 days ago",
			duration: 365 * 24 * time.Hour,
			minScore: 0.0,
			maxScore: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := Entry{
				Score:   1.0,
				LastHit: now.Add(-tt.duration),
			}
			score := WeightedScore(entry)
			assert.True(t, score >= tt.minScore && score <= tt.maxScore,
				"score %v not in range [%v, %v]", score, tt.minScore, tt.maxScore)
		})
	}
}

func TestWeightedScore_ZeroScore(t *testing.T) {
	t.Parallel()

	entry := Entry{
		Score:   0.0,
		Created: time.Now(),
	}

	assert.Equal(t, 0.0, WeightedScore(entry))
}

func TestWeightedScore_NegativeScoreAllowed(t *testing.T) {
	t.Parallel()

	entry := Entry{
		Score:   -5.0,
		Created: time.Now(),
	}

	assert.True(t, WeightedScore(entry) < 0)
}

func TestSortByWeightedScore_SortsDescending(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			ID:      "low",
			Score:   1.0,
			Created: now,
		},
		{
			ID:      "high",
			Score:   10.0,
			Created: now,
		},
		{
			ID:      "mid",
			Score:   5.0,
			Created: now,
		},
	}

	SortByWeightedScore(entries)

	assert.Equal(t, "high", entries[0].ID)
	assert.Equal(t, "mid", entries[1].ID)
	assert.Equal(t, "low", entries[2].ID)
}

func TestSortByWeightedScore_TiesBreakerByID(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			ID:      "zebra",
			Score:   1.0,
			Created: now,
		},
		{
			ID:      "apple",
			Score:   1.0,
			Created: now,
		},
		{
			ID:      "monkey",
			Score:   1.0,
			Created: now,
		},
	}

	SortByWeightedScore(entries)

	assert.Equal(t, "apple", entries[0].ID)
	assert.Equal(t, "monkey", entries[1].ID)
	assert.Equal(t, "zebra", entries[2].ID)
}

func TestSortByWeightedScore_MixedScoresAndRecency(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name    string
		entries []Entry
		want    []string
	}{
		{
			name: "high score beats recent low score (1 day decay)",
			entries: []Entry{
				{
					ID:      "old-high",
					Score:   10.0,
					LastHit: now.Add(-24 * time.Hour),
				},
				{
					ID:      "recent-low",
					Score:   1.0,
					LastHit: now,
				},
			},
			want: []string{"old-high", "recent-low"},
		},
		{
			name: "base score beats slight recency advantage",
			entries: []Entry{
				{
					ID:      "old-higher",
					Score:   5.0,
					LastHit: now.Add(-14 * 24 * time.Hour),
				},
				{
					ID:      "recent-lower",
					Score:   2.0,
					LastHit: now,
				},
			},
			want: []string{"old-higher", "recent-lower"},
		},
		{
			name: "stable sort with duplicate weighted scores in mixed dataset",
			entries: []Entry{
				{
					ID:      "high",
					Score:   10.0,
					LastHit: now,
				},
				{
					ID:      "tie-first",
					Score:   1.0,
					LastHit: now,
				},
				{
					ID:      "low",
					Score:   0.5,
					LastHit: now,
				},
				{
					ID:      "tie-second",
					Score:   1.0,
					LastHit: now,
				},
			},
			want: []string{"high", "tie-first", "tie-second", "low"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SortByWeightedScore(tt.entries)
			gotIDs := make([]string, len(tt.entries))
			for i, e := range tt.entries {
				gotIDs[i] = e.ID
			}
			assert.Equal(t, tt.want, gotIDs)
		})
	}
}

func TestSortByWeightedScore_EmptySlice(t *testing.T) {
	t.Parallel()

	var entries []Entry
	SortByWeightedScore(entries)
	assert.Equal(t, 0, len(entries))
}

func TestSortByWeightedScore_SingleEntry(t *testing.T) {
	t.Parallel()

	entries := []Entry{
		{
			ID:    "only",
			Score: 5.0,
		},
	}
	SortByWeightedScore(entries)
	assert.Equal(t, 1, len(entries))
	assert.Equal(t, "only", entries[0].ID)
}

func TestSortByWeightedScore_StableSort(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := []Entry{
		{
			ID:      "a",
			Score:   1.0,
			Created: now,
		},
		{
			ID:      "b",
			Score:   1.0,
			Created: now,
		},
		{
			ID:      "c",
			Score:   1.0,
			Created: now,
		},
	}

	SortByWeightedScore(entries)

	assert.Equal(t, []string{"a", "b", "c"}, []string{entries[0].ID, entries[1].ID, entries[2].ID})
}

func TestSortByWeightedScore_LargeDataset(t *testing.T) {
	t.Parallel()

	now := time.Now()
	entries := make([]Entry, 1000)
	for i := range 1000 {
		entries[i] = Entry{
			ID:      fmt.Sprintf("entry-%d", i),
			Score:   float64(1000 - i),
			Created: now.Add(-time.Duration(i) * time.Hour),
		}
	}

	SortByWeightedScore(entries)

	// check head, middle, and tail
	assert.True(t, entries[0].Score >= entries[1].Score, "head not sorted correctly")
	assert.True(t, entries[499].Score >= entries[500].Score, "middle not sorted correctly")
	assert.True(t, entries[998].Score >= entries[999].Score, "tail not sorted correctly")
}
