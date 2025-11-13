package tributarylog

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/nilpntr/tributary/tributarytype"
)

// ZapLogger wraps a zap.Logger to implement the tributarylog.Logger interface.
type ZapLogger struct {
	logger *zap.Logger
}

// NewZapLogger creates a new ZapLogger with the given configuration.
func NewZapLogger(config *LoggerConfig) (*ZapLogger, error) {
	if config == nil {
		config = DefaultConfig()
	}

	zapConfig := zap.NewProductionConfig()

	// Set log level
	switch config.Level {
	case LevelDebug:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case LevelInfo:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case LevelWarn:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case LevelError:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	case LevelOff:
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.FatalLevel + 1) // Disable all logs
	}

	// Configure structured/console output
	if !config.Structured {
		zapConfig.Encoding = "console"
	}

	// Configure timestamp
	if !config.IncludeTimestamp {
		zapConfig.EncoderConfig.TimeKey = ""
	} else if config.TimeFormat != "" {
		zapConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(config.TimeFormat)
	}

	// Configure caller information
	if config.IncludeCaller {
		zapConfig.EncoderConfig.CallerKey = "caller"
		zapConfig.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	} else {
		zapConfig.EncoderConfig.CallerKey = ""
	}

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, err
	}

	return &ZapLogger{logger: logger}, nil
}

// NewDevelopmentZapLogger creates a new development-oriented ZapLogger.
func NewDevelopmentZapLogger() (*ZapLogger, error) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return nil, err
	}
	return &ZapLogger{logger: logger}, nil
}

// NewProductionZapLogger creates a new production-oriented ZapLogger.
func NewProductionZapLogger() (*ZapLogger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}
	return &ZapLogger{logger: logger}, nil
}

// Debug logs a debug-level message with optional fields.
func (l *ZapLogger) Debug(msg string, fields ...tributarytype.Field) {
	l.logger.Debug(msg, l.convertFields(fields...)...)
}

// Info logs an info-level message with optional fields.
func (l *ZapLogger) Info(msg string, fields ...tributarytype.Field) {
	l.logger.Info(msg, l.convertFields(fields...)...)
}

// Warn logs a warning-level message with optional fields.
func (l *ZapLogger) Warn(msg string, fields ...tributarytype.Field) {
	l.logger.Warn(msg, l.convertFields(fields...)...)
}

// Error logs an error-level message with optional fields.
func (l *ZapLogger) Error(msg string, fields ...tributarytype.Field) {
	l.logger.Error(msg, l.convertFields(fields...)...)
}

// With returns a logger with the specified fields pre-populated.
func (l *ZapLogger) With(fields ...tributarytype.Field) tributarytype.Logger {
	return &ZapLogger{
		logger: l.logger.With(l.convertFields(fields...)...),
	}
}

// WithContext returns a logger with context information.
func (l *ZapLogger) WithContext(ctx interface{}) tributarytype.Logger {
	if c, ok := ctx.(context.Context); ok {
		fields := extractContextFields(c)
		if len(fields) == 0 {
			return l
		}
		return l.With(fields...)
	}
	return l
}

// convertFields converts tributarytype.Field slice to zap.Field slice.
func (l *ZapLogger) convertFields(fields ...tributarytype.Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, field := range fields {
		zapFields[i] = l.convertField(field)
	}
	return zapFields
}

// convertField converts a single tributarytype.Field to zap.Field.
func (l *ZapLogger) convertField(field tributarytype.Field) zap.Field {
	switch v := field.Value.(type) {
	case string:
		return zap.String(field.Key, v)
	case int:
		return zap.Int(field.Key, v)
	case int64:
		return zap.Int64(field.Key, v)
	case float64:
		return zap.Float64(field.Key, v)
	case bool:
		return zap.Bool(field.Key, v)
	case error:
		return zap.Error(v) // Use zap's special error handling
	case []byte:
		return zap.ByteString(field.Key, v)
	default:
		return zap.Any(field.Key, v)
	}
}

// extractContextFields extracts logging fields from context.
func extractContextFields(ctx context.Context) []tributarytype.Field {
	var fields []tributarytype.Field

	// Extract correlation ID if present
	if correlationID := ctx.Value("correlation_id"); correlationID != nil {
		if id, ok := correlationID.(string); ok {
			fields = append(fields, tributarytype.Field{Key: "correlation_id", Value: id})
		}
	}

	// Extract request ID if present
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			fields = append(fields, tributarytype.Field{Key: "request_id", Value: id})
		}
	}

	// Extract workflow execution ID if present
	if workflowID := ctx.Value("workflow_execution_id"); workflowID != nil {
		if id, ok := workflowID.(int64); ok {
			fields = append(fields, tributarytype.Field{Key: "workflow_execution_id", Value: id})
		}
	}

	// Extract step ID if present
	if stepID := ctx.Value("step_id"); stepID != nil {
		if id, ok := stepID.(int64); ok {
			fields = append(fields, tributarytype.Field{Key: "step_id", Value: id})
		}
	}

	// Extract user ID if present
	if userID := ctx.Value("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			fields = append(fields, tributarytype.Field{Key: "user_id", Value: id})
		}
	}

	return fields
}

// Close properly closes the underlying zap logger.
func (l *ZapLogger) Close() error {
	return l.logger.Sync()
}
