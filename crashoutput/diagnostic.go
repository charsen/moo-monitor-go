package crashoutput

import (
	"context"
	"errors"
	"runtime"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/records"
)

var ErrEmpty = errors.New("crash output contains no diagnostic")
var ErrChanged = errors.New("crash output changed after preparation")

type DiagnosticConfig struct {
	Env, Project, GoVersion, GoravelVersion string
	MaxReceipts                             int
}

// Prepared owns a fixed, masked diagnostic snapshot. Callers cannot mutate its
// timestamp/content; recovery after commitment uses the buffer's fixed image.
type Prepared struct {
	manager     *Manager
	identity    buffer.DiagnosticIdentity
	snapshot    records.Snapshot
	at          time.Time
	maxReceipts int
}

func (p *Prepared) ArtifactID() string { return p.identity.ArtifactID }
func (p *Prepared) RawDigest() string  { return p.identity.RawDigest }
func (m *Manager) Prepare(ctx context.Context, id string, cfg DiagnosticConfig, at time.Time) (*Prepared, error) {
	if !utf8.ValidString(cfg.Env) || !utf8.ValidString(cfg.Project) || !utf8.ValidString(cfg.GoVersion) || !utf8.ValidString(cfg.GoravelVersion) || cfg.Project == "" || at.Year() < 1 || at.Year() > 9999 || cfg.MaxReceipts < 0 || cfg.MaxReceipts > 100000 {
		return nil, ErrConfig
	}
	report, err := m.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if !report.OutputPresent {
		return nil, ErrEmpty
	}
	if cfg.GoVersion == "" {
		cfg.GoVersion = runtime.Version()
	}
	snapshot := records.Snapshot{Request: records.Request{Method: "CLI"}, Context: records.GoContext{Env: cfg.Env, Project: cfg.Project, Go: cfg.GoVersion, Goravel: cfg.GoravelVersion}, Trace: records.Trace{Full: report.MaskedText}}
	return &Prepared{manager: m, identity: buffer.DiagnosticIdentity{ArtifactID: id, RawDigest: report.Digest}, snapshot: snapshot, at: at, maxReceipts: cfg.MaxReceipts}, nil
}

// Apply rechecks the source artifact before entering the buffer's versioned
// journal. It does not claim a cross-filesystem transaction or delete raw data.
func (m *Manager) Apply(ctx context.Context, prepared *Prepared, target *buffer.Buffer) (buffer.DiagnosticResult, error) {
	if prepared == nil || prepared.manager != m || target == nil {
		return buffer.DiagnosticResult{}, ErrConfig
	}
	current, err := m.Inspect(ctx, prepared.identity.ArtifactID)
	if err != nil {
		return buffer.DiagnosticResult{}, err
	}
	if current.Digest != prepared.identity.RawDigest {
		return buffer.DiagnosticResult{}, ErrChanged
	}
	return target.ImportDiagnostic(ctx, buffer.DiagnosticInput{Identity: prepared.identity, Snapshot: prepared.snapshot, OccurredAt: prepared.at, MaxReceipts: prepared.maxReceipts})
}
