package promclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is a dedicated HTTP client for the Prometheus query API.
// It is the only place in the codebase that knows Prometheus's wire format
// (matrix/vector, [timestamp, "stringValue"] pairs, etc) — everything above
// this layer works with plain Go structs.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// rawResponse mirrors Prometheus's /api/v1/query and /api/v1/query_range envelope.
type rawResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string          `json:"resultType"` // "vector" | "matrix"
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// rawVectorSample is one element of a "vector" result (instant query).
type rawVectorSample struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"` // [unixSeconds float64, "value" string]
}

// rawMatrixSeries is one element of a "matrix" result (range query).
type rawMatrixSeries struct {
	Metric map[string]string `json:"metric"`
	Values [][2]interface{}  `json:"values"`
}

// Query runs an instant PromQL query and returns normalized samples.
func (c *Client) Query(ctx context.Context, promql string) ([]InstantSample, error) {
	body, err := c.do(ctx, "/api/v1/query", url.Values{"query": {promql}})
	if err != nil {
		return nil, err
	}

	var samples []rawVectorSample
	if err := json.Unmarshal(body.Data.Result, &samples); err != nil {
		return nil, fmt.Errorf("decode vector result: %w", err)
	}

	out := make([]InstantSample, 0, len(samples))
	for _, s := range samples {
		ts, val, err := decodePoint(s.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, InstantSample{Labels: s.Metric, Timestamp: ts, Value: val})
	}
	return out, nil
}

// QueryRange runs a ranged PromQL query and returns normalized series.
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step string) ([]Series, error) {
	body, err := c.do(ctx, "/api/v1/query_range", url.Values{
		"query": {promql},
		"start": {strconv.FormatInt(start.Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
		"step":  {step},
	})
	if err != nil {
		return nil, err
	}

	var raw []rawMatrixSeries
	if err := json.Unmarshal(body.Data.Result, &raw); err != nil {
		return nil, fmt.Errorf("decode matrix result: %w", err)
	}

	out := make([]Series, 0, len(raw))
	for _, s := range raw {
		samples := make([]Sample, 0, len(s.Values))
		for _, pair := range s.Values {
			ts, val, err := decodePoint(pair)
			if err != nil {
				return nil, err
			}
			samples = append(samples, Sample{Timestamp: ts, Value: val})
		}
		out = append(out, Series{Labels: s.Metric, Samples: samples})
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, path string, params url.Values) (*rawResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build prometheus request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed rawResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode prometheus envelope: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("prometheus error (%s): %s", parsed.ErrorType, parsed.Error)
	}
	return &parsed, nil
}

// decodePoint converts Prometheus's [unixSeconds, "stringValue"] pair into (time.Time, float64).
func decodePoint(pair [2]interface{}) (time.Time, float64, error) {
	secs, ok := pair[0].(float64)
	if !ok {
		return time.Time{}, 0, fmt.Errorf("unexpected timestamp type in prometheus response")
	}
	valStr, ok := pair[1].(string)
	if !ok {
		return time.Time{}, 0, fmt.Errorf("unexpected value type in prometheus response")
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("parse prometheus value: %w", err)
	}
	return time.Unix(int64(secs), 0), val, nil
}
