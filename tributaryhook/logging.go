package tributaryhook

import (
	"context"
	"fmt"
	"time"

	"github.com/nilpntr/tributary/tributarytype"
)

// LoggingHook provides lifecycle logging for steps.
type LoggingHook struct {
	BaseHook
	logger tributarytype.Logger
}

// NewLoggingHook creates a new logging hook with the given logger.
func NewLoggingHook(logger tributarytype.Logger) *LoggingHook {
	return &LoggingHook{
		logger: logger,
	}
}

// BeforeInsert logs step insertion events.
func (h *LoggingHook) BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	h.logger.Debug("Step about to be inserted",
		tributarytype.Field{Key: "step_kind", Value: args.Kind()},
		tributarytype.Field{Key: "args_size", Value: len(argsBytes)},
	)
	return argsBytes, nil
}

// AfterInsert logs successful step insertion.
func (h *LoggingHook) AfterInsert(ctx context.Context, stepID int64, args tributarytype.StepArgs) {
	h.logger.Info("Step inserted successfully",
		tributarytype.Field{Key: "step_id", Value: stepID},
		tributarytype.Field{Key: "step_kind", Value: args.Kind()},
	)
}

// BeforeWork logs step execution start.
func (h *LoggingHook) BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	h.logger.Info("Starting step execution",
		tributarytype.Field{Key: "step_id", Value: stepID},
		tributarytype.Field{Key: "step_kind", Value: kind},
		tributarytype.Field{Key: "start_time", Value: time.Now()},
	)
	return argsBytes, nil
}

// AfterWork logs step execution completion.
func (h *LoggingHook) AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	if err != nil {
		h.logger.Error("Step execution failed",
			tributarytype.Field{Key: "step_id", Value: stepID},
			tributarytype.Field{Key: "error", Value: err.Error()},
			tributarytype.Field{Key: "end_time", Value: time.Now()},
		)
	} else {
		h.logger.Info("Step execution completed successfully",
			tributarytype.Field{Key: "step_id", Value: stepID},
			tributarytype.Field{Key: "result_size", Value: len(result)},
			tributarytype.Field{Key: "end_time", Value: time.Now()},
		)
	}
	return result, nil
}

// TimingLoggingHook extends LoggingHook with execution timing information.
type TimingLoggingHook struct {
	BaseHook
	logger tributarytype.Logger
	timers map[int64]time.Time
}

// NewTimingLoggingHook creates a new timing logging hook.
func NewTimingLoggingHook(logger tributarytype.Logger) *TimingLoggingHook {
	return &TimingLoggingHook{
		logger: logger,
		timers: make(map[int64]time.Time),
	}
}

// BeforeWork logs step execution start and records timing.
func (h *TimingLoggingHook) BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	startTime := time.Now()
	h.timers[stepID] = startTime

	h.logger.Info("Starting step execution",
		tributarytype.Field{Key: "step_id", Value: stepID},
		tributarytype.Field{Key: "step_kind", Value: kind},
		tributarytype.Field{Key: "start_time", Value: startTime},
	)
	return argsBytes, nil
}

// AfterWork logs step execution completion with timing information.
func (h *TimingLoggingHook) AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	endTime := time.Now()

	var duration time.Duration
	if startTime, exists := h.timers[stepID]; exists {
		duration = endTime.Sub(startTime)
		delete(h.timers, stepID) // Clean up to prevent memory leaks
	}

	if err != nil {
		h.logger.Error("Step execution failed",
			tributarytype.Field{Key: "step_id", Value: stepID},
			tributarytype.Field{Key: "error", Value: err.Error()},
			tributarytype.Field{Key: "duration_ms", Value: duration.Milliseconds()},
			tributarytype.Field{Key: "end_time", Value: endTime},
		)
	} else {
		h.logger.Info("Step execution completed successfully",
			tributarytype.Field{Key: "step_id", Value: stepID},
			tributarytype.Field{Key: "result_size", Value: len(result)},
			tributarytype.Field{Key: "duration_ms", Value: duration.Milliseconds()},
			tributarytype.Field{Key: "end_time", Value: endTime},
		)
	}
	return result, nil
}

// StructuredLoggingHook provides rich structured logging with correlation IDs.
type StructuredLoggingHook struct {
	BaseHook
	logger tributarytype.Logger
}

// NewStructuredLoggingHook creates a new structured logging hook.
func NewStructuredLoggingHook(logger tributarytype.Logger) *StructuredLoggingHook {
	return &StructuredLoggingHook{
		logger: logger,
	}
}

// BeforeInsert logs step insertion with correlation context.
func (h *StructuredLoggingHook) BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	contextualLogger := h.logger.WithContext(ctx)
	contextualLogger.Debug("Step insertion starting",
		tributarytype.Field{Key: "step_kind", Value: args.Kind()},
		tributarytype.Field{Key: "args_size_bytes", Value: len(argsBytes)},
		tributarytype.Field{Key: "phase", Value: "before_insert"},
	)
	return argsBytes, nil
}

// AfterInsert logs successful step insertion with context.
func (h *StructuredLoggingHook) AfterInsert(ctx context.Context, stepID int64, args tributarytype.StepArgs) {
	contextualLogger := h.logger.WithContext(ctx)
	contextualLogger.Info("Step insertion completed",
		tributarytype.Field{Key: "step_id", Value: stepID},
		tributarytype.Field{Key: "step_kind", Value: args.Kind()},
		tributarytype.Field{Key: "phase", Value: "after_insert"},
	)
}

// BeforeWork logs step execution start with full context.
func (h *StructuredLoggingHook) BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	contextualLogger := h.logger.WithContext(ctx).With(
		tributarytype.Field{Key: "step_id", Value: stepID},
		tributarytype.Field{Key: "step_kind", Value: kind},
	)

	contextualLogger.Info("Step execution starting",
		tributarytype.Field{Key: "args_size_bytes", Value: len(argsBytes)},
		tributarytype.Field{Key: "phase", Value: "before_work"},
	)
	return argsBytes, nil
}

// AfterWork logs step execution completion with full context.
func (h *StructuredLoggingHook) AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	contextualLogger := h.logger.WithContext(ctx).With(
		tributarytype.Field{Key: "step_id", Value: stepID},
		tributarytype.Field{Key: "phase", Value: "after_work"},
	)

	if err != nil {
		contextualLogger.Error("Step execution failed",
			tributarytype.Field{Key: "error", Value: err.Error()},
			tributarytype.Field{Key: "error_type", Value: fmt.Sprintf("%T", err)},
		)
	} else {
		contextualLogger.Info("Step execution succeeded",
			tributarytype.Field{Key: "result_size_bytes", Value: len(result)},
		)
	}
	return result, nil
}
