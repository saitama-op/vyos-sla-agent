package metrics

import (
	"sort"
)

// Average calculates the mean of a float64 slice
func Average(data []float64) float64 {
	if len(data) == 0 {
		return 0.0
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// Percentile95 calculates the 95th percentile using the nearest-rank method
func Percentile95(data []float64) float64 {
	if len(data) == 0 {
		return 0.0
	}
	
	// Create a copy to avoid mutating the source slice during sort
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	index := int(float64(len(sorted)) * 0.95)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}
