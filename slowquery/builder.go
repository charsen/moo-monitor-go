package slowquery

import (
	"crypto/md5"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gitee.com/charsen/moo-go-monitor/mask"
)

var ErrInput = errors.New("invalid slow-query input")
var ErrConfiguration = errors.New("invalid slow-query configuration")
var sqlSpace = regexp.MustCompile(`[\t\n\f\r\x0b ]+`)
var sqlPlaceholders = regexp.MustCompile(`\(\s*\?(\s*,\s*\?)*\s*\)`)
var sqlNumbers = regexp.MustCompile(`\b[0-9]+\b`)

type Builder struct {
	config    Config
	masker    *mask.Masker
	threshold float64
}

func New(config Config) (*Builder, error) {
	threshold := float64(100)
	if config.ThresholdMillis != nil {
		threshold = *config.ThresholdMillis
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold >= math.Exp2(63) || !utf8.ValidString(config.BasePath) || !utf8.ValidString(config.Env) || !utf8.ValidString(config.Project) {
		return nil, ErrConfiguration
	}
	for _, pattern := range config.SkipPatterns {
		if !utf8.ValidString(pattern) {
			return nil, ErrConfiguration
		}
	}
	masker, err := mask.New(config.MaskKeys)
	if err != nil {
		return nil, ErrConfiguration
	}
	config.SkipPatterns = append([]string(nil), config.SkipPatterns...)
	config.MaskKeys = append([]string(nil), config.MaskKeys...)
	return &Builder{config: config, masker: masker, threshold: threshold}, nil
}
func (b *Builder) Accept(input Input) (bool, error) {
	if !b.config.Enabled {
		return false, nil
	}
	if err := validateInput(input); err != nil {
		return false, err
	}
	if input.TookMillis < b.threshold {
		return false, nil
	}
	for _, pattern := range b.config.SkipPatterns {
		if pattern != "" && strings.Contains(input.Raw, pattern) {
			return false, nil
		}
	}
	if _, err := fillBindings(input.Raw, input.Bindings); err != nil {
		return false, err
	}
	return true, nil
}
func normalizeSQL(raw string) string {
	result := sqlSpace.ReplaceAllString(raw, " ")
	result = sqlPlaceholders.ReplaceAllString(result, "(?)")
	result = sqlNumbers.ReplaceAllString(result, "N")
	if len(result) > 8192 {
		result = result[:8192]
	}
	return strings.Trim(result, " \t\n\r\x00\x0b")
}
func (b *Builder) relative(file string) string {
	if b.config.BasePath == "" {
		return file
	}
	base := strings.TrimRight(b.config.BasePath, "/")
	if strings.HasPrefix(file, base) {
		return strings.TrimLeft(file[len(base):], "/")
	}
	return file
}
func (b *Builder) Hash(input Input) string {
	identity := fmt.Sprintf("%s:%d|%s", b.relative(input.File), input.Line, normalizeSQL(input.Raw))
	return fmt.Sprintf("%x", md5.Sum([]byte(identity)))[:12]
}
func validateInput(input Input) error {
	if !utf8.ValidString(input.Raw) || !utf8.ValidString(input.File) || input.Line < 0 || input.TookMillis < 0 || math.IsNaN(input.TookMillis) || math.IsInf(input.TookMillis, 0) {
		return ErrInput
	}
	for _, value := range []*string{input.Connection, input.Request.URL, input.Request.IP, input.Request.UserID, input.Request.UserName} {
		if value != nil && !utf8.ValidString(*value) {
			return ErrInput
		}
	}
	if !utf8.ValidString(input.Request.Method) {
		return ErrInput
	}
	return nil
}
func capSQL(text string) string {
	if utf8.RuneCountInString(text) <= 4096 {
		return text
	}
	return string([]rune(text)[:4096]) + "…"
}
func (b *Builder) prepare(input Input, now time.Time) (*Record, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}
	last, err := fillBindings(input.Raw, input.Bindings)
	if err != nil || !utf8.ValidString(last) {
		return nil, ErrInput
	}
	raw := b.masker.MaskSecrets(b.masker.MaskSensitiveSQL(input.Raw))
	last = b.masker.MaskSecrets(b.masker.MaskSensitiveSQL(last))
	request := input.Request
	if request.Method == "" {
		request.Method = "CLI"
	}
	if request.URL != nil {
		text := b.masker.MaskURL(*request.URL)
		request.URL = &text
	}
	copyValue := func(value *string) *string {
		if value == nil {
			return nil
		}
		text := *value
		return &text
	}
	request.IP = copyValue(request.IP)
	request.UserID = copyValue(request.UserID)
	request.UserName = copyValue(request.UserName)
	connection := copyValue(input.Connection)
	iso := timestamp(now)
	return &Record{LastSeen: iso, SQL: SQL{Raw: capSQL(raw), Last: capSQL(last), RawBytes: int64(len(raw)), LastBytes: int64(len(last))}, Took: Took{LastMillis: roundMillis(input.TookMillis), MaxMillis: roundMillis(input.TookMillis), ThresholdMillis: int64(b.threshold)}, At: At{File: b.relative(input.File), Line: input.Line, Connection: connection}, Context: Context{Env: b.config.Env, Project: b.config.Project, OccurredAt: iso}, Request: request}, nil
}
func (b *Builder) Build(input Input, now time.Time) (*Record, error) {
	record, err := b.prepare(input, now)
	if err != nil {
		return nil, err
	}
	record.Hash = b.Hash(input)
	record.FirstSeen = record.LastSeen
	record.Count = 1
	record.Status = "open"
	record.Daily = Daily{Date: now.Format("2006-01-02"), Count: 1}
	return record, nil
}
func (b *Builder) Refresh(existing Record, input Input, now time.Time, overflow int64) (*Record, error) {
	extra := max(int64(0), overflow)
	if existing.Count < 1 || extra == math.MaxInt64 || existing.Count > math.MaxInt64-1-extra || existing.Daily.Count < 1 || existing.Daily.Date == now.Format("2006-01-02") && existing.Daily.Count == math.MaxInt64 {
		return nil, ErrInput
	}
	latest, err := b.prepare(input, now)
	if err != nil {
		return nil, err
	}
	existing.LastSeen = latest.LastSeen
	existing.Count += 1 + extra
	existing.SQL.Last = latest.SQL.Last
	existing.SQL.LastBytes = latest.SQL.LastBytes
	existing.Took.LastMillis = latest.Took.LastMillis
	existing.Took.MaxMillis = max(existing.Took.MaxMillis, latest.Took.LastMillis)
	existing.At = latest.At
	existing.Context = latest.Context
	existing.Request = latest.Request
	if existing.Daily.Date == now.Format("2006-01-02") {
		existing.Daily.Count++
	} else {
		existing.Daily = Daily{Date: now.Format("2006-01-02"), Count: 1}
	}
	return &existing, nil
}
