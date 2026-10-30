package cloudsync

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"time"

	"gitee.com/charsen/moo-go-monitor/buffer"
	"gitee.com/charsen/moo-go-monitor/records"
	"gitee.com/charsen/moo-go-monitor/slowquery"
)

var ErrSelfTest = errors.New("monitor self-test did not confirm storage")
var ErrSelfTestConfig = errors.New("invalid monitor self-test configuration")

type SelfTestType string

const (
	SelfTestBoth     SelfTestType = "both"
	SelfTestRuntimes SelfTestType = "runtimes"
	SelfTestSlowSQL  SelfTestType = "slow_sql"
)

type HealthClient interface {
	Heartbeater
	Send(context.Context, []buffer.Snapshot) (Receipt, error)
	SendSlow(context.Context, []buffer.SlowSnapshot) (Receipt, error)
}
type SelfTestConfig struct {
	Meta                    HeartbeatMeta
	Project, GoravelVersion string
	Clock                   func() time.Time
}
type SelfTestStep struct {
	Attempted, Saved bool
	Hash, Outcome    string
}
type SelfTestResult struct {
	Heartbeat                 bool
	Runtime, SlowSQL          SelfTestStep
	GoVersion, GoravelVersion string
}
type SelfTest struct {
	client  HealthClient
	config  SelfTestConfig
	runtime *records.Builder
	slow    *slowquery.Builder
}

func NewSelfTest(client HealthClient, config SelfTestConfig) (*SelfTest, error) {
	if client == nil || !validWireString(config.Project) || !validWireString(config.GoravelVersion) || !validWireString(config.Meta.AppEnv) || !validWireString(config.Meta.AppName) || !validWireString(config.Meta.SDKVersion) {
		return nil, ErrSelfTestConfig
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	builder, err := records.New(records.Config{})
	if err != nil {
		return nil, ErrSelfTestConfig
	}
	slow, err := slowquery.New(slowquery.Config{Enabled: true, Env: config.Meta.AppEnv, Project: config.Project})
	if err != nil {
		return nil, ErrSelfTestConfig
	}
	return &SelfTest{client: client, config: config, runtime: builder, slow: slow}, nil
}

// Run is explicitly network-writing. It never inserts into a local Buffer or ACK ledger.
func (s *SelfTest) Run(ctx context.Context, kind SelfTestType) (SelfTestResult, error) {
	result := SelfTestResult{GoVersion: runtime.Version(), GoravelVersion: s.config.GoravelVersion}
	if ctx == nil || (kind != SelfTestBoth && kind != SelfTestRuntimes && kind != SelfTestSlowSQL) {
		return result, ErrSelfTestConfig
	}
	if err := s.client.Heartbeat(ctx, s.config.Meta); err != nil {
		return result, errors.Join(ErrSelfTest, safeHealthCause(err))
	}
	result.Heartbeat = true
	now := s.config.Clock()
	stamp := now.Format("2006-01-02T15:04:05.000Z07:00")
	if strings.HasSuffix(stamp, "Z") {
		stamp = strings.TrimSuffix(stamp, "Z") + "+00:00"
	}
	failed := false
	if kind == SelfTestBoth || kind == SelfTestRuntimes {
		result.Runtime.Attempted = true
		record, err := s.runtime.Build(records.Snapshot{Exception: records.Exception{Class: "moo-go-monitor/SelfTestError", Message: "moo-monitor connectivity self-test; safe to ignore", File: "moo-monitor/self-test"}, Context: records.GoContext{Env: s.config.Meta.AppEnv, Project: s.config.Project, Go: runtime.Version(), Goravel: s.config.GoravelVersion}, Source: "self_test", Meta: map[string]any{"updated_at": stamp}}, now)
		if err != nil {
			result.Runtime.Outcome = "invalid_record"
			failed = true
		} else {
			result.Runtime.Hash = record.Hash
			batch := []buffer.Snapshot{{Hash: record.Hash, Record: *record}}
			receipt, err := s.client.Send(ctx, batch)
			result.Runtime.Saved, result.Runtime.Outcome = selfTestSaved(receipt, batch, err)
			failed = failed || !result.Runtime.Saved
		}
	}
	if kind == SelfTestBoth || kind == SelfTestSlowSQL {
		result.SlowSQL.Attempted = true
		record, err := s.slow.Build(slowquery.Input{Raw: "select /* moo-monitor self-test */ ? as connectivity_check", Bindings: []any{int64(1)}, TookMillis: 100, File: "moo-monitor/self-test"}, now)
		if err != nil {
			result.SlowSQL.Outcome = "invalid_record"
			failed = true
		} else {
			record.Meta = map[string]any{"updated_at": stamp}
			result.SlowSQL.Hash = record.Hash
			batch := []buffer.SlowSnapshot{{Hash: record.Hash, Record: *record}}
			receipt, err := s.client.SendSlow(ctx, batch)
			result.SlowSQL.Saved, result.SlowSQL.Outcome = selfTestSaved(receipt, batch, err)
			failed = failed || !result.SlowSQL.Saved
		}
	}
	if failed {
		if ctx.Err() != nil {
			return result, errors.Join(ErrSelfTest, ctx.Err())
		}
		return result, ErrSelfTest
	}
	return result, nil
}
func selfTestSaved[R any](receipt Receipt, batch []buffer.VersionedSnapshot[R], failure error) (bool, string) {
	if failure != nil {
		return false, "transport_or_receipt"
	}
	if receipt.Legacy || validate(receipt, batch) != nil || len(receipt.Results) != 1 {
		return false, "invalid_receipt"
	}
	if receipt.Saved == 1 && receipt.Filtered == 0 && receipt.Skipped == 0 && receipt.Results[0].Status == "saved" {
		return true, "saved"
	}
	item := receipt.Results[0]
	if item.Status == "filtered" {
		return false, "filtered"
	}
	if item.Retryable {
		return false, "retryable"
	}
	return false, "rejected"
}

// Explicit self-test reports keep safe categories, never arbitrary Client error text or URL.
func safeHealthCause(err error) error {
	for _, category := range []error{context.Canceled, context.DeadlineExceeded, ErrHTTPConfig, ErrWire, ErrHTTPTransport, ErrHTTPStatus, ErrHTTPReceipt, ErrResponseTooLarge} {
		if errors.Is(err, category) {
			return category
		}
	}
	return ErrSelfTest
}
