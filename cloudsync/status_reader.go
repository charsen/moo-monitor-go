package cloudsync

import (
	"context"
	"errors"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
	"gitee.com/charsen/moo-monitor-go/slowquery"
)

// StatusReader owns scope handles, never a Client. Construction can prepare lock
// files; Stats itself never creates or changes state or sends network requests.
type StatusReader struct {
	runtime *synchronizer[records.Record]
	slow    *synchronizer[slowquery.Record]
}

func NewStatusReader(runtime *buffer.Buffer, slow *buffer.SlowBuffer) (*StatusReader, error) {
	reader := &StatusReader{}
	var err error
	if runtime != nil {
		reader.runtime, err = newStateAccess[records.Record](runtime, stateFile, lockFile)
		if err != nil {
			return nil, err
		}
	}
	if slow != nil {
		reader.slow, err = newStateAccess[slowquery.Record](slow, slowStateFile, slowLockFile)
		if err != nil {
			if reader.runtime != nil {
				err = errors.Join(err, reader.runtime.Close())
			}
			return nil, err
		}
	}
	return reader, nil
}
func (r *StatusReader) Stats(ctx context.Context, target PushTarget) (PushPendingStats, error) {
	result := PushPendingStats{Runtimes: TypePendingStats{Skipped: "not_selected"}, SlowSQL: TypePendingStats{Skipped: "not_selected"}}
	if ctx == nil || (target != PushBoth && target != PushRuntimes && target != PushSlowSQL) {
		return result, ErrPushConfig
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var failure error
	if target == PushBoth || target == PushRuntimes {
		if r.runtime == nil {
			result.Runtimes.Skipped = "source_unavailable"
			result.Runtimes.Reason = "unavailable"
		} else {
			result.Runtimes.Skipped = ""
			value, err := r.runtime.Stats(ctx)
			result.Runtimes.PendingStats = value
			failure = errors.Join(failure, err)
		}
	}
	if target == PushBoth || target == PushSlowSQL {
		if r.slow == nil {
			result.SlowSQL.Skipped = "source_unavailable"
			result.SlowSQL.Reason = "unavailable"
		} else {
			result.SlowSQL.Skipped = ""
			value, err := r.slow.Stats(ctx)
			result.SlowSQL.PendingStats = value
			failure = errors.Join(failure, err)
		}
	}
	return result, failure
}
func (r *StatusReader) Close() error {
	var failure error
	if r.runtime != nil {
		failure = errors.Join(failure, r.runtime.Close())
	}
	if r.slow != nil {
		failure = errors.Join(failure, r.slow.Close())
	}
	return failure
}
