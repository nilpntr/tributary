package tributaryhook

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/nilpntr/tributary/tributarytype"
)

// MetricsCollector defines the interface for collecting metrics.
type MetricsCollector interface {
	// IncrementCounter increments a counter metric.
	IncrementCounter(name string, labels map[string]string)

	// RecordGauge records a gauge metric value.
	RecordGauge(name string, value float64, labels map[string]string)

	// RecordHistogram records a histogram metric value.
	RecordHistogram(name string, value float64, labels map[string]string)

	// RecordTiming records a timing metric.
	RecordTiming(name string, duration time.Duration, labels map[string]string)
}

// MetricsHook provides metrics collection for step lifecycle events.
type MetricsHook struct {
	BaseHook
	collector MetricsCollector
	timers    map[int64]time.Time
}

// NewMetricsHook creates a new metrics hook with the given collector.
func NewMetricsHook(collector MetricsCollector) *MetricsHook {
	return &MetricsHook{
		collector: collector,
		timers:    make(map[int64]time.Time),
	}
}

// BeforeInsert records step insertion metrics.
func (h *MetricsHook) BeforeInsert(_ context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	labels := map[string]string{
		"step_kind": args.Kind(),
	}

	h.collector.IncrementCounter("tributary_steps_inserted_total", labels)
	h.collector.RecordGauge("tributary_step_args_size_bytes", float64(len(argsBytes)), labels)

	return argsBytes, nil
}

// BeforeWork records step execution start metrics.
func (h *MetricsHook) BeforeWork(_ context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	startTime := time.Now()
	h.timers[stepID] = startTime

	labels := map[string]string{
		"step_kind": kind,
	}

	h.collector.IncrementCounter("tributary_steps_started_total", labels)
	h.collector.RecordGauge("tributary_step_args_size_bytes", float64(len(argsBytes)), labels)

	return argsBytes, nil
}

// AfterWork records step execution completion metrics.
func (h *MetricsHook) AfterWork(_ context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	endTime := time.Now()

	// Calculate duration if we have a start time
	var duration time.Duration
	if startTime, exists := h.timers[stepID]; exists {
		duration = endTime.Sub(startTime)
		delete(h.timers, stepID) // Clean up to prevent memory leaks
	}

	labels := map[string]string{
		"status": "success",
	}

	if err != nil {
		labels["status"] = "failure"
		h.collector.IncrementCounter("tributary_steps_failed_total", labels)
	} else {
		h.collector.IncrementCounter("tributary_steps_completed_total", labels)
		h.collector.RecordGauge("tributary_step_result_size_bytes", float64(len(result)), labels)
	}

	if duration > 0 {
		h.collector.RecordTiming("tributary_step_execution_duration", duration, labels)
		h.collector.RecordHistogram("tributary_step_execution_duration_seconds", duration.Seconds(), labels)
	}

	return result, nil
}

// InMemoryMetricsCollector is a simple in-memory metrics collector for testing.
type InMemoryMetricsCollector struct {
	counters   map[string]int64
	gauges     map[string]float64
	histograms map[string][]float64
	timings    map[string][]time.Duration
}

// NewInMemoryMetricsCollector creates a new in-memory metrics collector.
func NewInMemoryMetricsCollector() *InMemoryMetricsCollector {
	return &InMemoryMetricsCollector{
		counters:   make(map[string]int64),
		gauges:     make(map[string]float64),
		histograms: make(map[string][]float64),
		timings:    make(map[string][]time.Duration),
	}
}

// IncrementCounter increments a counter metric.
func (c *InMemoryMetricsCollector) IncrementCounter(name string, labels map[string]string) {
	key := buildMetricKey(name, labels)
	val := c.counters[key]
	atomic.AddInt64(&val, 1)
}

// RecordGauge records a gauge metric value.
func (c *InMemoryMetricsCollector) RecordGauge(name string, value float64, labels map[string]string) {
	key := buildMetricKey(name, labels)
	c.gauges[key] = value
}

// RecordHistogram records a histogram metric value.
func (c *InMemoryMetricsCollector) RecordHistogram(name string, value float64, labels map[string]string) {
	key := buildMetricKey(name, labels)
	c.histograms[key] = append(c.histograms[key], value)
}

// RecordTiming records a timing metric.
func (c *InMemoryMetricsCollector) RecordTiming(name string, duration time.Duration, labels map[string]string) {
	key := buildMetricKey(name, labels)
	c.timings[key] = append(c.timings[key], duration)
}

// GetCounter returns the current value of a counter metric.
func (c *InMemoryMetricsCollector) GetCounter(name string, labels map[string]string) int64 {
	key := buildMetricKey(name, labels)
	val := c.counters[key]
	return atomic.LoadInt64(&val)
}

// GetGauge returns the current value of a gauge metric.
func (c *InMemoryMetricsCollector) GetGauge(name string, labels map[string]string) float64 {
	key := buildMetricKey(name, labels)
	return c.gauges[key]
}

// GetHistogram returns all recorded values for a histogram metric.
func (c *InMemoryMetricsCollector) GetHistogram(name string, labels map[string]string) []float64 {
	key := buildMetricKey(name, labels)
	values := c.histograms[key]
	result := make([]float64, len(values))
	copy(result, values)
	return result
}

// GetTimings returns all recorded timings for a timing metric.
func (c *InMemoryMetricsCollector) GetTimings(name string, labels map[string]string) []time.Duration {
	key := buildMetricKey(name, labels)
	values := c.timings[key]
	result := make([]time.Duration, len(values))
	copy(result, values)
	return result
}

// Reset clears all metrics.
func (c *InMemoryMetricsCollector) Reset() {
	c.counters = make(map[string]int64)
	c.gauges = make(map[string]float64)
	c.histograms = make(map[string][]float64)
	c.timings = make(map[string][]time.Duration)
}

// buildMetricKey creates a consistent key for metrics with labels.
func buildMetricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	key := name
	for k, v := range labels {
		key += "," + k + "=" + v
	}
	return key
}
