package cordis

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode identifies a class of framework errors, mirroring the codes used
// by the TypeScript implementation.
type ErrorCode string

// ErrorCode values defined by the core framework.
const (
	// ErrCodeInactiveEffect is returned when an effect, listener, service or
	// plugin is registered on a context whose fiber is no longer active.
	ErrCodeInactiveEffect ErrorCode = "INACTIVE_EFFECT"
)

// Error is the framework error type. It carries a stable Code that callers
// can match with errors.As, plus a human readable message.
type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

// ErrInactiveEffect is returned by effect, listener, service and plugin
// registration when the receiving context belongs to an inactive fiber.
var ErrInactiveEffect = &Error{
	Code:    ErrCodeInactiveEffect,
	Message: "cannot create effect on inactive context",
}

// IsInactiveEffect reports whether err is or wraps an inactive-effect error.
func IsInactiveEffect(err error) bool {
	ce, ok := errors.AsType[*Error](err)
	return ok && ce.Code == ErrCodeInactiveEffect
}

// Issue describes a single config validation problem, mirroring a Standard
// Schema issue from the TypeScript implementation.
type Issue struct {
	Path    []string
	Message string
}

// ValidationError is returned when plugin config validation fails.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	b.WriteString("invalid config:")
	for _, issue := range e.Issues {
		if len(issue.Path) > 0 {
			fmt.Fprintf(&b, "\n  - %s (at %s)", issue.Message, strings.Join(issue.Path, "."))
		} else {
			fmt.Fprintf(&b, "\n  - %s", issue.Message)
		}
	}
	return b.String()
}
