package cloudsync

import "context"

// SelectionMode names the candidate policy, independently of transport.
type SelectionMode string

const Incremental SelectionMode = "incremental"

type PreviewOptions struct{ Mode SelectionMode }

type TypePreview struct {
	TypePendingStats
	// Nil means unknown, never a known empty candidate set.
	Selected, WouldSend *int
}
type PushPreview struct{ Runtimes, SlowSQL TypePreview }

func previewResult(value PushPendingStats) PushPreview {
	convert := func(stats TypePendingStats) TypePreview {
		result := TypePreview{TypePendingStats: stats}
		if stats.Known && stats.Counts != nil {
			selected := stats.Counts.PendingOpen + stats.Counts.PendingResolved
			result.Selected = &selected
			result.WouldSend = &selected
		}
		return result
	}
	return PushPreview{Runtimes: convert(value.Runtimes), SlowSQL: convert(value.SlowSQL)}
}

// Preview uses the same locked, strict, read-only classification as Stats. It
// predicts local incremental candidates, not Cloud acceptance or configuration.
func (r *StatusReader) Preview(ctx context.Context, target PushTarget, options PreviewOptions) (PushPreview, error) {
	if options.Mode != Incremental {
		return PushPreview{}, ErrPushConfig
	}
	value, err := r.Stats(ctx, target)
	return previewResult(value), err
}

func (s *synchronizer[R]) Preview(ctx context.Context, options PreviewOptions) (TypePreview, error) {
	if options.Mode != Incremental {
		return TypePreview{}, ErrPushConfig
	}
	value, err := s.Stats(ctx)
	return previewResult(PushPendingStats{Runtimes: TypePendingStats{PendingStats: value}}).Runtimes, err
}

func (p *Pusher) Preview(ctx context.Context, target PushTarget, options PreviewOptions) (PushPreview, error) {
	if options.Mode != Incremental {
		return PushPreview{}, ErrPushConfig
	}
	value, err := p.Stats(ctx, target)
	return previewResult(value), err
}
