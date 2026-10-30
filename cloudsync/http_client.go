package cloudsync

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"gitee.com/charsen/moo-go-monitor/buffer"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const runtimeIntakePath = "api/v1/runtimes/intake"
const runtimeDiscardPath = "api/v1/runtimes/discard-local"

var (
	ErrHTTPConfig       = errors.New("invalid Cloud HTTP configuration")
	ErrWire             = errors.New("Cloud record cannot be safely JSON encoded")
	ErrHTTPTransport    = errors.New("Cloud HTTP connection failed")
	ErrHTTPStatus       = errors.New("Cloud HTTP status rejected")
	ErrHTTPReceipt      = errors.New("Cloud HTTP receipt rejected")
	ErrResponseTooLarge = errors.New("Cloud HTTP response too large")
)

type HTTPConfig struct {
	BaseURL, Token   string
	Timeout          time.Duration
	VerifyTLS        *bool
	MaxResponseBytes int64
	Client           *http.Client
}

type HTTPError struct {
	Operation       string
	Status, Attempt int
	category, cause error
}

func (e *HTTPError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Operation, e.category.Error(), e.Status)
	}
	return e.Operation + ": " + e.category.Error()
}
func (e *HTTPError) Unwrap() error        { return e.cause }
func (e *HTTPError) Is(target error) bool { return target == e.category }

type HTTPClient struct {
	baseURL, token   string
	client           *http.Client
	owned            *http.Transport
	maxResponseBytes int64
}

func NewHTTPClient(config HTTPConfig) (*HTTPClient, error) {
	base, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, ErrHTTPConfig
	}
	base.Scheme = strings.ToLower(base.Scheme)
	if strings.Contains(config.BaseURL, "#") || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || base.RawFragment != "" || base.Opaque != "" || (base.Scheme != "http" && base.Scheme != "https") || config.Token == "" || !validWireString(config.Token) {
		return nil, ErrHTTPConfig
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if timeout < 0 {
		return nil, ErrHTTPConfig
	}
	limit := config.MaxResponseBytes
	if limit == 0 {
		limit = 1 << 20
	}
	if limit < 1 || limit == math.MaxInt64 {
		return nil, ErrHTTPConfig
	}
	if config.Client != nil && config.VerifyTLS != nil {
		return nil, ErrHTTPConfig
	} // Borrowed transport owns its TLS policy.
	client := http.Client{}
	if config.Client != nil {
		client = *config.Client
	}
	var owned *http.Transport
	if client.Transport == nil {
		verify := true
		if config.VerifyTLS != nil {
			verify = *config.VerifyTLS
		}
		owned = &http.Transport{DialContext: (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 10, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: timeout, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !verify}}
		client.Transport = owned
	}
	client.Timeout = timeout
	client.Jar = nil // This API authenticates in the body, never through unrelated borrowed cookies.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTPClient{baseURL: strings.TrimRight(base.String(), "/"), token: config.Token, client: &client, owned: owned, maxResponseBytes: limit}, nil
}

// CloseIdleConnections only touches the transport created by this SDK, never a borrowed owner.
func (c *HTTPClient) CloseIdleConnections() {
	if c.owned != nil {
		c.owned.CloseIdleConnections()
	}
}

func (c *HTTPClient) Send(ctx context.Context, batch []buffer.Snapshot) (Receipt, error) {
	if len(batch) == 0 {
		return Receipt{Legacy: true}, nil
	}
	body, err := encodeRuntimeBody(c.token, batch)
	if err != nil {
		return Receipt{}, &HTTPError{Operation: "runtime intake", category: ErrWire, cause: err}
	}
	data, status, attempt, err := c.post(ctx, runtimeIntakePath, body, 3, 200*time.Millisecond)
	if err != nil {
		return Receipt{}, err
	}
	receipt, err := DecodeReceipt(status, data, batch)
	if err != nil {
		return Receipt{}, &HTTPError{Operation: "runtime intake", Status: status, Attempt: attempt, category: ErrHTTPReceipt, cause: err}
	}
	return receipt, nil
}

// DiscardLocalNoise only sends the Cloud protocol. It never deletes local buffer state.
func (c *HTTPClient) DiscardLocalNoise(ctx context.Context) (int64, error) {
	return c.discardLocal(ctx, runtimeDiscardPath, "discard local")
}
func (c *HTTPClient) discardLocal(ctx context.Context, path, operation string) (int64, error) {
	body, err := encodeDiscardBody(c.token)
	if err != nil {
		return 0, &HTTPError{Operation: operation, category: ErrWire, cause: err}
	}
	data, status, attempt, err := c.post(ctx, path, body, 2, 100*time.Millisecond)
	if err != nil {
		return 0, err
	}
	deleted, err := decodeDiscard(data)
	if err != nil {
		return 0, &HTTPError{Operation: operation, Status: status, Attempt: attempt, category: ErrHTTPReceipt, cause: err}
	}
	return deleted, nil
}

func (c *HTTPClient) post(ctx context.Context, path string, body []byte, attempts int, delay time.Duration) ([]byte, int, int, error) {
	operation := "runtime intake"
	switch path {
	case runtimeDiscardPath:
		operation = "discard local"
	case slowIntakePath:
		operation = "slow-query intake"
	case slowDiscardPath:
		operation = "discard slow-query local"
	}
	if ctx == nil {
		return nil, 0, 0, &HTTPError{Operation: operation, category: ErrHTTPConfig}
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, 0, attempt, &HTTPError{Operation: operation, Attempt: attempt, category: ErrHTTPTransport, cause: err}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+path, bytes.NewReader(body))
		if err != nil {
			return nil, 0, attempt, &HTTPError{Operation: operation, Attempt: attempt, category: ErrHTTPConfig, cause: err}
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Content-Type", "application/json")
		response, err := c.client.Do(request)
		if err == nil {
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				response.Body.Close()
				return nil, response.StatusCode, attempt, &HTTPError{Operation: operation, Status: response.StatusCode, Attempt: attempt, category: ErrHTTPStatus}
			}
			data, readErr := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
			response.Body.Close()
			if int64(len(data)) > c.maxResponseBytes {
				return nil, response.StatusCode, attempt, &HTTPError{Operation: operation, Status: response.StatusCode, Attempt: attempt, category: ErrResponseTooLarge}
			}
			if readErr == nil {
				return data, response.StatusCode, attempt, nil
			}
			err = readErr
		} else if response != nil && response.Body != nil {
			response.Body.Close()
		}
		if ctx.Err() != nil {
			return nil, 0, attempt, &HTTPError{Operation: operation, Attempt: attempt, category: ErrHTTPTransport, cause: ctx.Err()}
		}
		if attempt == attempts || !connectionFailure(err) {
			return nil, 0, attempt, &HTTPError{Operation: operation, Attempt: attempt, category: ErrHTTPTransport, cause: err}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, 0, attempt, &HTTPError{Operation: operation, Attempt: attempt, category: ErrHTTPTransport, cause: ctx.Err()}
		case <-timer.C:
		}
	}
	return nil, 0, attempts, &HTTPError{Operation: operation, category: ErrHTTPTransport}
}

func connectionFailure(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var certificate *tls.CertificateVerificationError
	if errors.As(err, &certificate) {
		return false
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
