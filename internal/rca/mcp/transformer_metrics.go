// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// MetricsTransformer transforms resource metrics into markdown analysis.
type MetricsTransformer struct{}

type metricStats struct {
	Mean                   float64
	Median                 float64
	Min                    float64
	Max                    float64
	StdDev                 float64
	CoefficientOfVariation float64
	P90                    float64
	P95                    float64
	StartTime              string
	EndTime                string
}

type anomalyInfo struct {
	SpikeCount        int
	MaxSpikeMagnitude float64
	LargestDrop       float64
}

type resourcePressure struct {
	AvgUsageToRequestRatio float64
	AvgUsageToLimitRatio   float64
	ExceededRequests       bool
	ExceededLimits         bool
}

func (t *MetricsTransformer) Transform(content map[string]any) (string, error) {
	// Extract time-series data
	metricsData := make(map[string][]float64)
	timestamps := make(map[string][]string)

	metricNames := []string{"cpuUsage", "cpuRequests", "cpuLimits", "memory", "memoryRequests", "memoryLimits"}

	for _, name := range metricNames {
		if data, ok := content[name].([]any); ok && len(data) > 0 {
			var values []float64
			var times []string

			for _, point := range data {
				if p, ok := point.(map[string]any); ok {
					if v, ok := p["value"].(float64); ok {
						values = append(values, v)
					}
					if t, ok := p["time"].(string); ok {
						times = append(times, t)
					}
				}
			}

			if len(values) > 0 {
				metricsData[name] = values
				timestamps[name] = times
			}
		}
	}

	if len(metricsData) == 0 {
		return "No metrics data available", nil
	}

	// Calculate statistics for usage metrics
	stats := make(map[string]*metricStats)
	anomalies := make(map[string]*anomalyInfo)
	configValues := make(map[string]float64)

	for name, values := range metricsData {
		switch name {
		case "cpuUsage", "memory":
			stats[name] = calculateStats(values, timestamps[name])
			anomalies[name] = detectAnomalies(values)
		case "cpuRequests", "cpuLimits", "memoryRequests", "memoryLimits":
			if len(values) > 0 {
				configValues[name] = values[0]
			}
		}
	}

	// Calculate resource pressure
	var cpuPressure, memoryPressure *resourcePressure

	if cpuUsage, ok := metricsData["cpuUsage"]; ok {
		cpuPressure = calculateResourcePressure(
			cpuUsage,
			metricsData["cpuRequests"],
			metricsData["cpuLimits"],
		)
	}

	if memory, ok := metricsData["memory"]; ok {
		memoryPressure = calculateResourcePressure(
			memory,
			metricsData["memoryRequests"],
			metricsData["memoryLimits"],
		)
	}

	// Calculate correlations
	var cpuMemoryCorr *float64
	if cpuUsage, ok := metricsData["cpuUsage"]; ok {
		if memory, ok := metricsData["memory"]; ok {
			corr := calculateCorrelation(cpuUsage, memory)
			cpuMemoryCorr = &corr
		}
	}

	// Build output
	return formatMetricsOutput(stats, anomalies, configValues, cpuPressure, memoryPressure, cpuMemoryCorr), nil
}

func calculateStats(values []float64, times []string) *metricStats {
	if len(values) == 0 {
		return nil
	}

	mean := calculateMean(values)
	stdDev := calculateStdDev(values, mean)

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	stats := &metricStats{
		Mean:   mean,
		Median: calculateMedian(sorted),
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		StdDev: stdDev,
		P90:    calculatePercentile(sorted, 90),
		P95:    calculatePercentile(sorted, 95),
	}

	if mean != 0 {
		stats.CoefficientOfVariation = stdDev / mean
	}

	if len(times) > 0 {
		stats.StartTime = times[0]
		stats.EndTime = times[len(times)-1]
	}

	return stats
}

func detectAnomalies(values []float64) *anomalyInfo {
	if len(values) < 2 {
		return &anomalyInfo{}
	}

	mean := calculateMean(values)
	stdDev := calculateStdDev(values, mean)

	const threshold = 3.0
	spikeCount := 0
	maxSpikeMagnitude := 0.0

	for _, v := range values {
		if stdDev > 0 {
			zScore := math.Abs((v - mean) / stdDev)
			if zScore > threshold {
				spikeCount++
			}
			if zScore > maxSpikeMagnitude {
				maxSpikeMagnitude = zScore
			}
		}
	}

	// Check for large percentage changes
	for i := 1; i < len(values); i++ {
		if values[i-1] != 0 {
			pctChange := math.Abs((values[i] - values[i-1]) / values[i-1]) * 100
			if pctChange > 50 {
				spikeCount++
			}
		}
	}

	// Find largest drop
	largestDrop := 0.0
	for i := 1; i < len(values); i++ {
		change := values[i] - values[i-1]
		if change < largestDrop {
			largestDrop = change
		}
	}

	return &anomalyInfo{
		SpikeCount:        spikeCount,
		MaxSpikeMagnitude: maxSpikeMagnitude,
		LargestDrop:       largestDrop,
	}
}

func calculateResourcePressure(usage, requests, limits []float64) *resourcePressure {
	if len(usage) == 0 {
		return nil
	}

	pressure := &resourcePressure{}

	if len(requests) > 0 {
		minLen := min(len(usage), len(requests))
		sum := 0.0
		for i := 0; i < minLen; i++ {
			if requests[i] > 0 {
				sum += usage[i] / requests[i]
			}
			if usage[i] > requests[i] {
				pressure.ExceededRequests = true
			}
		}
		pressure.AvgUsageToRequestRatio = sum / float64(minLen)
	}

	if len(limits) > 0 {
		minLen := min(len(usage), len(limits))
		sum := 0.0
		for i := 0; i < minLen; i++ {
			if limits[i] > 0 {
				sum += usage[i] / limits[i]
			}
			if usage[i] > limits[i] {
				pressure.ExceededLimits = true
			}
		}
		pressure.AvgUsageToLimitRatio = sum / float64(minLen)
	}

	return pressure
}

func calculateCorrelation(x, y []float64) float64 {
	minLen := min(len(x), len(y))
	if minLen < 2 {
		return 0
	}

	x = x[:minLen]
	y = y[:minLen]

	meanX := calculateMean(x)
	meanY := calculateMean(y)

	var sumXY, sumX2, sumY2 float64
	for i := 0; i < minLen; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		sumXY += dx * dy
		sumX2 += dx * dx
		sumY2 += dy * dy
	}

	if sumX2 == 0 || sumY2 == 0 {
		return 0
	}

	return sumXY / math.Sqrt(sumX2*sumY2)
}

func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(len(values)))
}

func calculateMedian(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func calculatePercentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	if lower == upper {
		return sorted[lower]
	}
	return sorted[lower] + (index-float64(lower))*(sorted[upper]-sorted[lower])
}

func formatMetricsOutput(
	stats map[string]*metricStats,
	anomalies map[string]*anomalyInfo,
	configValues map[string]float64,
	cpuPressure, memoryPressure *resourcePressure,
	cpuMemoryCorr *float64,
) string {
	var sb strings.Builder

	sb.WriteString("## Resource Metrics Analysis\n")

	if s, ok := stats["cpuUsage"]; ok && s != nil {
		sb.WriteString(fmt.Sprintf("Time Range: %s to %s\n\n", s.StartTime, s.EndTime))
	}

	// CPU Metrics
	sb.WriteString("### CPU Metrics\n\n")

	if s, ok := stats["cpuUsage"]; ok && s != nil {
		sb.WriteString("**CPU Usage:**\n")
		sb.WriteString(fmt.Sprintf("- Mean: %.4f cores (%.2fm)\n", s.Mean, s.Mean*1000))
		sb.WriteString(fmt.Sprintf("- Median: %.4f cores\n", s.Median))
		sb.WriteString(fmt.Sprintf("- Range: %.4f - %.4f cores\n", s.Min, s.Max))
		sb.WriteString(fmt.Sprintf("- Std Dev: %.4f, CV: %.2f\n", s.StdDev, s.CoefficientOfVariation))
		sb.WriteString(fmt.Sprintf("- P90: %.4f cores, P95: %.4f cores\n", s.P90, s.P95))

		if a, ok := anomalies["cpuUsage"]; ok && a.SpikeCount > 0 {
			sb.WriteString(fmt.Sprintf("- **Anomalies Detected:** %d spike(s)\n", a.SpikeCount))
			sb.WriteString(fmt.Sprintf("  - Max spike magnitude: %.2f σ (standard deviations)\n", a.MaxSpikeMagnitude))
		}
		sb.WriteString("\n")
	}

	if v, ok := configValues["cpuRequests"]; ok {
		sb.WriteString("**CPU Requests (configured):**\n")
		sb.WriteString(fmt.Sprintf("- Value: %.4f cores (%.2fm)\n\n", v, v*1000))
	}

	if v, ok := configValues["cpuLimits"]; ok {
		sb.WriteString("**CPU Limits (configured):**\n")
		sb.WriteString(fmt.Sprintf("- Value: %.4f cores (%.2fm)\n\n", v, v*1000))
	}

	if cpuPressure != nil {
		sb.WriteString("**CPU Resource Pressure:**\n")
		sb.WriteString(fmt.Sprintf("- Usage to Request ratio: %.2f%%\n", cpuPressure.AvgUsageToRequestRatio*100))
		sb.WriteString(fmt.Sprintf("- Usage to Limit ratio: %.2f%%\n", cpuPressure.AvgUsageToLimitRatio*100))
		if cpuPressure.ExceededRequests {
			sb.WriteString("- **CPU usage exceeded requests at some point**\n")
		}
		if cpuPressure.ExceededLimits {
			sb.WriteString("- **CPU usage exceeded limits at some point (throttling likely occurred)**\n")
		}
		sb.WriteString("\n")
	}

	// Memory Metrics
	sb.WriteString("### Memory Metrics\n\n")

	if s, ok := stats["memory"]; ok && s != nil {
		sb.WriteString("**Memory Usage:**\n")
		sb.WriteString(fmt.Sprintf("- Mean: %.0f bytes (%.2f MB)\n", s.Mean, s.Mean/1024/1024))
		sb.WriteString(fmt.Sprintf("- Median: %.0f bytes (%.2f MB)\n", s.Median, s.Median/1024/1024))
		sb.WriteString(fmt.Sprintf("- Range: %.0f - %.0f bytes (%.2f - %.2f MB)\n", s.Min, s.Max, s.Min/1024/1024, s.Max/1024/1024))
		sb.WriteString(fmt.Sprintf("- Std Dev: %.0f bytes, CV: %.2f\n", s.StdDev, s.CoefficientOfVariation))
		sb.WriteString(fmt.Sprintf("- P90: %.2f MB, P95: %.2f MB\n", s.P90/1024/1024, s.P95/1024/1024))

		if a, ok := anomalies["memory"]; ok && a.SpikeCount > 0 {
			sb.WriteString(fmt.Sprintf("- **Anomalies Detected:** %d spike(s)\n", a.SpikeCount))
			sb.WriteString(fmt.Sprintf("  - Max spike magnitude: %.2f σ\n", a.MaxSpikeMagnitude))
			if a.LargestDrop < -1000000 {
				sb.WriteString(fmt.Sprintf("  - Largest drop: %.2f MB\n", a.LargestDrop/1024/1024))
			}
		}
		sb.WriteString("\n")
	}

	if v, ok := configValues["memoryRequests"]; ok {
		sb.WriteString("**Memory Requests (configured):**\n")
		sb.WriteString(fmt.Sprintf("- Value: %.0f bytes (%.2f MB)\n\n", v, v/1024/1024))
	}

	if v, ok := configValues["memoryLimits"]; ok {
		sb.WriteString("**Memory Limits (configured):**\n")
		sb.WriteString(fmt.Sprintf("- Value: %.0f bytes (%.2f MB)\n\n", v, v/1024/1024))
	}

	if memoryPressure != nil {
		sb.WriteString("**Memory Resource Pressure:**\n")
		sb.WriteString(fmt.Sprintf("- Usage to Request ratio: %.2f%%\n", memoryPressure.AvgUsageToRequestRatio*100))
		sb.WriteString(fmt.Sprintf("- Usage to Limit ratio: %.2f%%\n", memoryPressure.AvgUsageToLimitRatio*100))
		if memoryPressure.ExceededRequests {
			sb.WriteString("- **Memory usage exceeded requests at some point**\n")
		}
		if memoryPressure.ExceededLimits {
			sb.WriteString("- **Memory usage exceeded limits at some point (OOM risk/occurred)**\n")
		}
		sb.WriteString("\n")
	}

	// Correlations
	if cpuMemoryCorr != nil {
		sb.WriteString("### Correlations\n")
		sb.WriteString(fmt.Sprintf("- CPU Usage vs Memory: %.3f\n\n", *cpuMemoryCorr))
	}

	return sb.String()
}

func init() {
	RegisterTransformer("get_component_resource_metrics", &MetricsTransformer{})
}
