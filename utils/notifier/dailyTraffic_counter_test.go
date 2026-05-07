package notifier

import (
	"testing"
	"time"

	"github.com/komari-monitor/komari/database/models"
)

const mebibyte = 1 << 20

func TestCalculateCounterUsageIgnoresRecoveringDip(t *testing.T) {
	t.Parallel()

	got := calculateCounterUsage(100*mebibyte, []int64{
		120 * mebibyte,
		140 * mebibyte,
		130 * mebibyte,
		150 * mebibyte,
	})

	want := int64(50 * mebibyte)
	if got != want {
		t.Fatalf("calculateCounterUsage() = %d, want %d", got, want)
	}
}

func TestCalculateCounterUsageIgnoresTrailingDip(t *testing.T) {
	t.Parallel()

	got := calculateCounterUsage(100*mebibyte, []int64{
		120 * mebibyte,
		140 * mebibyte,
		130 * mebibyte,
	})

	want := int64(40 * mebibyte)
	if got != want {
		t.Fatalf("calculateCounterUsage() = %d, want %d", got, want)
	}
}

func TestCalculateCounterUsageCountsConfirmedReset(t *testing.T) {
	t.Parallel()

	got := calculateCounterUsage(100*mebibyte, []int64{
		120 * mebibyte,
		140 * mebibyte,
		10 * mebibyte,
		30 * mebibyte,
		55 * mebibyte,
		80 * mebibyte,
	})

	want := int64(120 * mebibyte)
	if got != want {
		t.Fatalf("calculateCounterUsage() = %d, want %d", got, want)
	}
}

func TestSortDailyTrafficRecordsUsesCounterTieBreakers(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
	records := []models.Record{
		{Time: models.FromTime(ts), NetTotalUp: 200, NetTotalDown: 200},
		{Time: models.FromTime(ts.Add(time.Minute)), NetTotalUp: 300, NetTotalDown: 300},
		{Time: models.FromTime(ts), NetTotalUp: 100, NetTotalDown: 100},
	}

	sortDailyTrafficRecords(records)

	if records[0].NetTotalUp != 100 || records[1].NetTotalUp != 200 || records[2].NetTotalUp != 300 {
		t.Fatalf("unexpected record order after sort: %+v", records)
	}
}
