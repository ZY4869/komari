package notifier

import (
	"sort"

	"github.com/komari-monitor/komari/database/models"
)

const (
	minSamplesForCounterReset = 3
	minCounterResetGrowth     = 32 << 20 // 32 MiB
)

func sortDailyTrafficRecords(records []models.Record) {
	sort.Slice(records, func(i, j int) bool {
		leftTime := records[i].Time.ToTime()
		rightTime := records[j].Time.ToTime()
		if leftTime.Equal(rightTime) {
			if records[i].NetTotalUp == records[j].NetTotalUp {
				return records[i].NetTotalDown < records[j].NetTotalDown
			}
			return records[i].NetTotalUp < records[j].NetTotalUp
		}
		return leftTime.Before(rightTime)
	})
}

func calculateCounterUsage(previous int64, values []int64) int64 {
	baseline := previous
	segmentMax := previous
	var total int64

	for idx, current := range values {
		if current >= segmentMax {
			segmentMax = current
			continue
		}
		if !isConfirmedCounterReset(segmentMax, values[idx:]) {
			continue
		}
		total += positiveCounterDelta(segmentMax, baseline)
		baseline = 0
		segmentMax = current
	}

	return total + positiveCounterDelta(segmentMax, baseline)
}

func isConfirmedCounterReset(previousMax int64, remaining []int64) bool {
	if len(remaining) < minSamplesForCounterReset {
		return false
	}

	current := remaining[0]
	if current >= previousMax {
		return false
	}

	futureMax := current
	for _, next := range remaining[1:] {
		if next > futureMax {
			futureMax = next
		}
		if futureMax >= previousMax {
			return false
		}
	}

	return futureMax-current >= minCounterResetGrowth
}

func positiveCounterDelta(current, previous int64) int64 {
	if current <= previous {
		return 0
	}
	return current - previous
}
