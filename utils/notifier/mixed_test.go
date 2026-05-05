package notifier

import "testing"

func TestRequiredMixedRecords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		total    int
		ratio    float32
		expected int
	}{
		{name: "zero ratio uses one record", total: 5, ratio: 0, expected: 1},
		{name: "round up ratio", total: 5, ratio: 0.6, expected: 3},
		{name: "single record minimum", total: 1, ratio: 0.9, expected: 1},
		{name: "cannot exceed total", total: 3, ratio: 2, expected: 3},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := requiredMixedRecords(tt.total, tt.ratio)
			if got != tt.expected {
				t.Fatalf("requiredMixedRecords(%d, %f) = %d, want %d", tt.total, tt.ratio, got, tt.expected)
			}
		})
	}
}

func TestTrafficDeltaHandlesReset(t *testing.T) {
	t.Parallel()

	if got := trafficDelta(100, 160); got != 60 {
		t.Fatalf("trafficDelta normal case = %d, want 60", got)
	}
	if got := trafficDelta(100, 20); got != 20 {
		t.Fatalf("trafficDelta reset case = %d, want 20", got)
	}
}
