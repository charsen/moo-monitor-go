package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"gitee.com/charsen/moo-monitor-go/buffer"
	"gitee.com/charsen/moo-monitor-go/slowquery"
)

const slowIntakePath = "api/v1/slow-queries/intake"
const slowDiscardPath = "api/v1/slow-queries/discard-local"
const slowStateFile = "sync-slow-queries.json"
const slowLockFile = ".sync-slow-queries.lock"

type SlowClient interface {
	SendSlow(context.Context, []buffer.SlowSnapshot) (Receipt, error)
}
type SlowSyncer struct {
	*synchronizer[slowquery.Record]
}

// This adapter bridges the two explicit client actions to the one confirmation coordinator.
type slowClient struct{ client SlowClient }

func (c slowClient) Send(ctx context.Context, batch []buffer.SlowSnapshot) (Receipt, error) {
	return c.client.SendSlow(ctx, batch)
}
func NewSlow(store *buffer.SlowBuffer, client SlowClient, config Config) (*SlowSyncer, error) {
	if store == nil || client == nil {
		return nil, errors.New("sync requires explicit slow-query buffer and client")
	}
	common, err := newSynchronizer[slowquery.Record](store, slowClient{client}, config, slowStateFile, slowLockFile)
	if err != nil {
		return nil, err
	}
	return &SlowSyncer{common}, nil
}
func (c *HTTPClient) SendSlow(ctx context.Context, batch []buffer.SlowSnapshot) (Receipt, error) {
	if len(batch) == 0 {
		return Receipt{Legacy: true}, nil
	}
	body, err := encodeSlowBody(c.token, batch)
	if err != nil {
		return Receipt{}, &HTTPError{Operation: "slow-query intake", category: ErrWire, cause: err}
	}
	data, status, attempt, err := c.post(ctx, slowIntakePath, body, 3, 200*time.Millisecond)
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := decodeReceipt(status, data, batch)
	if err != nil {
		return Receipt{}, &HTTPError{Operation: "slow-query intake", Status: status, Attempt: attempt, category: ErrHTTPReceipt, cause: err}
	}
	return receipt, nil
}

// DiscardSlowLocalNoise is transport only; local pending cleanup is a separate explicit action.
func (c *HTTPClient) DiscardSlowLocalNoise(ctx context.Context) (int64, error) {
	return c.discardLocal(ctx, slowDiscardPath, "discard slow-query local")
}
func encodeSlowBody(token string, batch []buffer.SlowSnapshot) ([]byte, error) {
	values := make([]slowquery.Record, 0, len(batch))
	for _, snapshot := range batch {
		r := snapshot.Record
		texts := []string{r.Hash, r.FirstSeen, r.LastSeen, r.Status, r.SQL.Raw, r.SQL.Last, r.At.File, r.Context.Env, r.Context.Project, r.Context.OccurredAt, r.Request.Method}
		for _, pointer := range []*string{r.ResolvedAt, r.ResolvedBy, r.ResolvedNote, r.At.Connection, r.Request.URL, r.Request.IP, r.Request.UserID, r.Request.UserName} {
			if pointer != nil {
				texts = append(texts, *pointer)
			}
		}
		for _, text := range texts {
			if !validWireString(text) {
				return nil, ErrWire
			}
		}
		if math.IsNaN(r.Took.LastMillis) || math.IsNaN(r.Took.MaxMillis) || math.IsInf(r.Took.LastMillis, 0) || math.IsInf(r.Took.MaxMillis, 0) {
			return nil, ErrWire
		}
		if err := validateWireValue(r.Meta, 0); err != nil {
			return nil, ErrWire
		}
		values = append(values, r)
	}
	body, err := json.Marshal(struct {
		Token   string             `json:"token"`
		Records []slowquery.Record `json:"records"`
	}{Token: token, Records: values})
	if err != nil {
		return nil, ErrWire
	}
	return body, nil
}
