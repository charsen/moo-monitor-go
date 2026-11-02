package capture

import "strings"

type HTTPStatusCoder interface{ HTTPStatusCode() int }
type ConsoleInputError interface{ ConsoleInputError() bool }

// A nil Config.Filter uses both skips; an explicit value supplies both decisions.
type FilterConfig struct{ SkipConsoleInput, SkipCLIExperiment bool }

func (c *Collector) shouldSkip(event error, options Options) bool {
	cli := options.Request.Method == "" || strings.EqualFold(options.Request.Method, "CLI")
	if cli && c.filter.SkipCLIExperiment && options.CLIExperiment {
		return true
	}
	current := event
	for inspected := 0; current != nil && inspected < 8; inspected++ {
		if status, ok := current.(HTTPStatusCoder); ok {
			code := status.HTTPStatusCode()
			return code >= 100 && code <= 599 && code < 500
		}
		if cli && c.filter.SkipConsoleInput {
			if input, ok := current.(ConsoleInputError); ok && input.ConsoleInputError() {
				return true
			}
		}
		single, ok := current.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		current = single.Unwrap()
	}
	return false
}
