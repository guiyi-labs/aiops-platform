package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLokiTimeout    = 15 * time.Second
	defaultLokiMaxEntries = 500
	defaultLokiMaxBytes   = 1 << 20
	// lokiResponseLimit caps the HTTP response body (2 MiB), allowing headroom
	// for JSON overhead while the per-query MaxBytes bound trims the entries.
	lokiResponseLimit = 2 << 20
)

// Compile-time assertion that LokiLogProvider implements LogProvider.
var _ LogProvider = (*LokiLogProvider)(nil)

// LokiConfig is the server-configured configuration for a LokiLogProvider.
// Endpoint is an HTTPS URL; credentials are supplied out-of-band and never
// appear in query input or results.
type LokiConfig struct {
	Endpoint       string
	RequestTimeout time.Duration
	MaxEntries     int
	MaxBytes       int
}

// LokiLogProvider implements LogProvider against a Loki-compatible HTTP API.
// It maps the fixed LogQuery shape to LogQL internally; the rendered LogQL is
// never exposed to callers.
type LokiLogProvider struct {
	config LokiConfig
	client *http.Client
}

// NewLokiLogProvider validates the endpoint, applies defaults and returns a
// bounded provider. Returns an error if the endpoint is not an absolute HTTPS
// URL without userinfo.
func NewLokiLogProvider(config LokiConfig) (*LokiLogProvider, error) {
	if err := requireHTTPSURL(config.Endpoint); err != nil {
		return nil, fmt.Errorf("loki: %w", err)
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = defaultLokiTimeout
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultLokiMaxEntries
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = defaultLokiMaxBytes
	}
	return &LokiLogProvider{
		config: config,
		client: newBoundedHTTPClient(config.RequestTimeout),
	}, nil
}

// Name returns the provider identifier.
func (p *LokiLogProvider) Name() string { return "loki" }

// QueryLogs validates the query, renders the fixed LogQL and dispatches a
// bounded range query. Provider failures are reported as StateUnavailable with
// a sanitized Error; hitting the entry or byte limit yields StateTruncated.
func (p *LokiLogProvider) QueryLogs(ctx context.Context, query LogQuery) (LogResult, error) {
	if err := validateLogQuery(query); err != nil {
		return LogResult{}, err
	}

	limit := query.Limit
	if limit == 0 {
		limit = p.config.MaxEntries
	}
	if limit > p.config.MaxEntries {
		limit = p.config.MaxEntries
	}
	maxBytes := query.MaxBytes
	if maxBytes == 0 {
		maxBytes = p.config.MaxBytes
	}

	logql := renderLogQL(query)

	requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()

	entries, truncated, err := p.queryRange(requestCtx, logql, query, limit)
	if err != nil {
		return LogResult{State: StateUnavailable, Error: sanitizeError(err)}, nil
	}

	entries, byteTruncated := enforceByteLimit(entries, maxBytes)
	result := LogResult{
		Entries:       entries,
		TotalReturned: len(entries),
	}
	if truncated || byteTruncated {
		result.State = StateTruncated
	} else {
		result.State = StateComplete
	}
	return result, nil
}

// renderLogQL builds the LogQL selector and optional line filter from the
// validated query. All interpolated values are escaped so client text cannot
// break out of the LogQL string literals — clients never supply raw LogQL.
func renderLogQL(query LogQuery) string {
	var builder strings.Builder
	builder.WriteByte('{')
	builder.WriteString(`cluster_id="`)
	builder.WriteString(strconv.FormatInt(query.ClusterID, 10))
	builder.WriteByte('"')
	builder.WriteByte(',')
	builder.WriteString(`namespace="`)
	builder.WriteString(escapeLogQLString(query.Namespace))
	builder.WriteByte('"')
	if query.PodName != "" {
		builder.WriteString(`,pod="`)
		builder.WriteString(escapeLogQLString(query.PodName))
		builder.WriteByte('"')
	}
	if query.Container != "" {
		builder.WriteString(`,container="`)
		builder.WriteString(escapeLogQLString(query.Container))
		builder.WriteByte('"')
	}
	builder.WriteByte('}')
	if query.TextFilter != "" {
		builder.WriteString(` |= "`)
		builder.WriteString(escapeLogQLString(query.TextFilter))
		builder.WriteByte('"')
	}
	return builder.String()
}

// escapeLogQLString escapes backslashes and double quotes so the value cannot
// break out of a LogQL double-quoted string literal.
func escapeLogQLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func (p *LokiLogProvider) queryRange(ctx context.Context, logql string, query LogQuery, limit int) ([]LogEntry, bool, error) {
	params := url.Values{}
	params.Set("query", logql)
	params.Set("start", strconv.FormatInt(query.Start.UnixNano(), 10))
	params.Set("end", strconv.FormatInt(query.End.UnixNano(), 10))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("direction", query.Direction)

	requestURL := strings.TrimRight(p.config.Endpoint, "/") + "/loki/api/v1/query_range?" + params.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, int64(lokiResponseLimit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > lokiResponseLimit {
		return nil, false, fmt.Errorf("loki response exceeds %d bytes", lokiResponseLimit)
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("loki returned status %d", response.StatusCode)
	}

	var apiResponse lokiAPIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, false, fmt.Errorf("decode loki response: %w", err)
	}
	if apiResponse.Status != "success" {
		return nil, false, fmt.Errorf("loki query failed: %s", apiResponse.Error)
	}

	entries := make([]LogEntry, 0, limit)
	truncated := false
	for _, stream := range apiResponse.Data.Result {
		for _, sample := range stream.Values {
			if len(entries) >= limit {
				truncated = true
				break
			}
			entry, ok := parseLokiSample(sample, stream.Stream)
			if !ok {
				continue
			}
			entries = append(entries, entry)
		}
		if truncated {
			break
		}
	}
	return entries, truncated, nil
}

type lokiAPIResponse struct {
	Status string      `json:"status"`
	Data   lokiAPIData `json:"data"`
	Error  string      `json:"error,omitempty"`
}

type lokiAPIData struct {
	Result []lokiAPIStream `json:"result"`
}

type lokiAPIStream struct {
	Stream map[string]string `json:"stream"`
	Values []lokiSample      `json:"values"`
}

// lokiSample is a [nanosecond-timestamp, line] pair from the Loki result. The
// timestamp is a JSON string of nanoseconds since epoch; the line is a string.
type lokiSample [2]any

func parseLokiSample(sample lokiSample, stream map[string]string) (LogEntry, bool) {
	var timestampText string
	switch v := sample[0].(type) {
	case string:
		timestampText = v
	case float64:
		timestampText = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return LogEntry{}, false
	}
	tsNano, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(timestampText, 64)
		if ferr != nil {
			return LogEntry{}, false
		}
		tsNano = int64(f)
	}
	line, ok := sample[1].(string)
	if !ok {
		return LogEntry{}, false
	}
	return LogEntry{
		Timestamp: time.Unix(0, tsNano).UTC(),
		Namespace: stream["namespace"],
		Pod:       stream["pod"],
		Container: stream["container"],
		Stream:    stream["stream"],
		Line:      line,
	}, true
}

// enforceByteLimit trims entries so the approximate total byte size does not
// exceed maxBytes, returning a truncated flag when trimming occurs.
func enforceByteLimit(entries []LogEntry, maxBytes int) ([]LogEntry, bool) {
	total := 0
	for i, entry := range entries {
		// Approximate per-entry size: line length plus fixed overhead for
		// timestamp and label resolution.
		size := len(entry.Line) + 64
		if total+size > maxBytes {
			return entries[:i], true
		}
		total += size
	}
	return entries, false
}
