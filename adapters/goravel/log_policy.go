package goravel

import (
	"errors"
	"github.com/goravel/framework/contracts/log"
)

var ErrLogPolicy = errors.New("invalid monitor log capture policy")

// LogPolicy applies only to monitor capture, never the delegate's native policy.
// A nil policy preserves legacy capture at every level >= Error.
type LogPolicy struct {
	ContextEnabled bool
	MessageEnabled bool
	Levels         []log.Level
}
type logPolicy struct {
	context, message bool
	levels           map[log.Level]bool
}

func copyLogPolicy(input *LogPolicy) (*logPolicy, error) {
	if input == nil {
		return nil, nil
	}
	policy := &logPolicy{context: input.ContextEnabled, message: input.MessageEnabled, levels: map[log.Level]bool{}}
	for _, level := range input.Levels {
		switch level {
		case log.LevelDebug, log.LevelInfo, log.LevelWarning, log.LevelError, log.LevelFatal, log.LevelPanic:
		default:
			return nil, ErrLogPolicy
		}
		policy.levels[level] = true
	}
	return policy, nil
}
func (h *Handler) captureEnabled(level log.Level) bool {
	if h.policy == nil {
		return level >= log.LevelError
	}
	return (h.policy.context || h.policy.message) && h.policy.levels[level]
}
func (h *Handler) sourceEnabled(contextError bool) bool {
	if h.policy == nil {
		return true
	}
	if contextError {
		return h.policy.context
	}
	return h.policy.message
}
