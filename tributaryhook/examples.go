package tributaryhook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nilpntr/tributary/tributarytype"
)

// ValidationHook demonstrates input validation before step insertion.
type ValidationHook struct {
	BaseHook
	validators map[string]func([]byte) error
}

// NewValidationHook creates a new validation hook.
func NewValidationHook() *ValidationHook {
	return &ValidationHook{
		validators: make(map[string]func([]byte) error),
	}
}

// AddValidator adds a validator function for a specific step kind.
func (h *ValidationHook) AddValidator(stepKind string, validator func([]byte) error) {
	h.validators[stepKind] = validator
}

// BeforeInsert validates step arguments before insertion.
func (h *ValidationHook) BeforeInsert(_ context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	if validator, exists := h.validators[args.Kind()]; exists {
		if err := validator(argsBytes); err != nil {
			return nil, fmt.Errorf("validation failed for step kind %s: %w", args.Kind(), err)
		}
	}
	return argsBytes, nil
}

// RetryPolicyHook demonstrates custom retry logic based on step types.
type RetryPolicyHook struct {
	BaseHook
	retryPolicies map[string]RetryPolicy
}

// RetryPolicy defines retry behavior for specific step types.
type RetryPolicy struct {
	MaxAttempts     int
	BackoffBase     time.Duration
	BackoffFactor   float64
	RetryableErrors []string
}

// NewRetryPolicyHook creates a new retry policy hook.
func NewRetryPolicyHook() *RetryPolicyHook {
	return &RetryPolicyHook{
		retryPolicies: make(map[string]RetryPolicy),
	}
}

// AddRetryPolicy adds a retry policy for a specific step kind.
func (h *RetryPolicyHook) AddRetryPolicy(stepKind string, policy RetryPolicy) {
	h.retryPolicies[stepKind] = policy
}

// AfterWork determines if a step should be retried based on the error and policy.
func (h *RetryPolicyHook) AfterWork(_ context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	if err == nil {
		return result, nil
	}

	// This is a demonstration - in practice, you'd need access to step information
	// to determine the retry policy. This hook would need to be enhanced to work
	// with the actual retry logic in the executor.

	return result, err
}

// AuditHook demonstrates audit logging for compliance.
type AuditHook struct {
	BaseHook
	auditLog func(event AuditEvent)
}

// AuditEvent represents an auditable event.
type AuditEvent struct {
	EventType string
	StepID    int64
	StepKind  string
	UserID    string
	Timestamp time.Time
	Details   map[string]interface{}
}

// NewAuditHook creates a new audit hook.
func NewAuditHook(auditLog func(AuditEvent)) *AuditHook {
	return &AuditHook{
		auditLog: auditLog,
	}
}

// BeforeInsert logs step insertion for audit trail.
func (h *AuditHook) BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	// Extract user ID from context if available
	userID := "unknown"
	if uid := ctx.Value("user_id"); uid != nil {
		if uidStr, ok := uid.(string); ok {
			userID = uidStr
		}
	}

	h.auditLog(AuditEvent{
		EventType: "step_insert",
		StepKind:  args.Kind(),
		UserID:    userID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"args_size": len(argsBytes),
		},
	})

	return argsBytes, nil
}

// AfterWork logs step completion for audit trail.
func (h *AuditHook) AfterWork(_ context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	status := "success"
	var details map[string]interface{}

	if err != nil {
		status = "failure"
		details = map[string]interface{}{
			"error": err.Error(),
		}
	} else {
		details = map[string]interface{}{
			"result_size": len(result),
		}
	}

	h.auditLog(AuditEvent{
		EventType: "step_complete",
		StepID:    stepID,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"status":  status,
			"details": details,
		},
	})

	return result, err
}

// CircuitBreakerHook demonstrates circuit breaker pattern for step types.
type CircuitBreakerHook struct {
	BaseHook
	circuitBreakers map[string]*CircuitBreaker
}

// CircuitBreaker implements a simple circuit breaker.
type CircuitBreaker struct {
	failures    int
	threshold   int
	timeout     time.Duration
	lastFailure time.Time
	state       CircuitBreakerState
}

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// NewCircuitBreakerHook creates a new circuit breaker hook.
func NewCircuitBreakerHook() *CircuitBreakerHook {
	return &CircuitBreakerHook{
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
}

// AddCircuitBreaker adds a circuit breaker for a specific step kind.
func (h *CircuitBreakerHook) AddCircuitBreaker(stepKind string, threshold int, timeout time.Duration) {
	h.circuitBreakers[stepKind] = &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
		state:     CircuitBreakerClosed,
	}
}

// BeforeWork checks circuit breaker state before execution.
func (h *CircuitBreakerHook) BeforeWork(_ context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	if breaker, exists := h.circuitBreakers[kind]; exists {
		if !breaker.AllowRequest() {
			return nil, fmt.Errorf("circuit breaker open for step kind %s", kind)
		}
	}
	return argsBytes, nil
}

// AfterWork updates circuit breaker state after execution.
func (h *CircuitBreakerHook) AfterWork(_ context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	// Note: This would need step kind information to work properly
	// This is a simplified demonstration

	return result, err
}

// AllowRequest checks if the circuit breaker allows the request.
func (cb *CircuitBreaker) AllowRequest() bool {
	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = CircuitBreakerHalfOpen
			return true
		}
		return false
	case CircuitBreakerHalfOpen:
		return true
	}
	return false
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.failures = 0
	cb.state = CircuitBreakerClosed
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.threshold {
		cb.state = CircuitBreakerOpen
	}
}

// FilteringHook demonstrates conditional hook execution based on step properties.
type FilteringHook struct {
	BaseHook
	wrapped Hook
	filter  func(stepKind string) bool
}

// NewFilteringHook creates a hook that only executes for steps matching the filter.
func NewFilteringHook(wrapped Hook, filter func(stepKind string) bool) *FilteringHook {
	return &FilteringHook{
		wrapped: wrapped,
		filter:  filter,
	}
}

// BeforeInsert conditionally calls the wrapped hook.
func (h *FilteringHook) BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	if !h.filter(args.Kind()) {
		return argsBytes, nil
	}
	return h.wrapped.BeforeInsert(ctx, args, argsBytes)
}

// AfterInsert conditionally calls the wrapped hook.
func (h *FilteringHook) AfterInsert(ctx context.Context, stepID int64, args tributarytype.StepArgs) {
	if h.filter(args.Kind()) {
		h.wrapped.AfterInsert(ctx, stepID, args)
	}
}

// BeforeWork conditionally calls the wrapped hook.
func (h *FilteringHook) BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	if !h.filter(kind) {
		return argsBytes, nil
	}
	return h.wrapped.BeforeWork(ctx, stepID, kind, argsBytes)
}

// AfterWork conditionally calls the wrapped hook.
func (h *FilteringHook) AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	// Note: This would need step kind information to work properly
	return h.wrapped.AfterWork(ctx, stepID, result, err)
}

// Helper functions for common filters

// KindFilter creates a filter that matches specific step kinds.
func KindFilter(kinds ...string) func(string) bool {
	kindSet := make(map[string]bool)
	for _, kind := range kinds {
		kindSet[kind] = true
	}
	return func(kind string) bool {
		return kindSet[kind]
	}
}

// PrefixFilter creates a filter that matches step kinds with specific prefixes.
func PrefixFilter(prefixes ...string) func(string) bool {
	return func(kind string) bool {
		for _, prefix := range prefixes {
			if strings.HasPrefix(kind, prefix) {
				return true
			}
		}
		return false
	}
}

// SuffixFilter creates a filter that matches step kinds with specific suffixes.
func SuffixFilter(suffixes ...string) func(string) bool {
	return func(kind string) bool {
		for _, suffix := range suffixes {
			if strings.HasSuffix(kind, suffix) {
				return true
			}
		}
		return false
	}
}
