package cloudsync

import "gitee.com/charsen/moo-monitor-go/buffer"

// All is explicit live-version replay. It ignores historical ACK selection,
// never exact local suppression, and is never used by an automatic Tick.
const All SelectionMode = "all"

type SyncOptions struct{ Mode SelectionMode }

type selectionDisposition uint8

const (
	selectionSelected selectionDisposition = iota
	selectionConfirmed
	selectionSuppressed
)

func validSelectionMode(mode SelectionMode) bool { return mode == Incremental || mode == All }

// Classification and operation selection differ: normal statistics still give
// confirmed precedence, while explicit replay always protects exact suppression.
func selectionReason[R any](snapshot buffer.VersionedSnapshot[R], confirmed Confirmed, suppressed suppressedVersion, mode SelectionMode) selectionDisposition {
	if mode == Incremental && matches(confirmed, snapshot) {
		return selectionConfirmed
	}
	if suppressionMatches(suppressed, snapshot) {
		return selectionSuppressed
	}
	return selectionSelected
}
