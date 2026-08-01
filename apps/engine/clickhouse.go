package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type chClient struct {
	baseURL    string
	authHeader string
	database   string
	http       *http.Client
}

func openClickHouse() (*chClient, error) {
	host := envOr("CLICKHOUSE_HOST", "localhost")
	port := envOr("CLICKHOUSE_PORT", "8443")
	user := envOr("CLICKHOUSE_USER", "default")
	password := os.Getenv("CLICKHOUSE_PASSWORD")
	database := envOr("CLICKHOUSE_DATABASE", "insightiq")
	secure := envOr("CLICKHOUSE_SECURE", "true") == "true"

	scheme := "https"
	if !secure {
		scheme = "http"
	}
	client := &chClient{
		baseURL:    fmt.Sprintf("%s://%s:%s", scheme, host, port),
		authHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)),
		database:   database,
		http:       &http.Client{Timeout: 90 * time.Second},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var one uint8
	if err := client.QueryRow(ctx, `SELECT 1`, &one); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}
	return client, nil
}

func (c *chClient) Close() error { return nil }

func (c *chClient) execQuery(ctx context.Context, query string) ([]byte, error) {
	q := url.Values{}
	q.Set("database", c.database)
	q.Set("default_format", "JSON")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/?"+q.Encode(), strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("clickhouse %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

type chJSON struct {
	Data []map[string]any `json:"data"`
}

func (c *chClient) QueryMaps(ctx context.Context, query string) ([]map[string]any, error) {
	body, err := c.execQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	var parsed chJSON
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode json: %w; body=%s", err, truncate(string(body), 200))
	}
	return parsed.Data, nil
}

func (c *chClient) QueryRow(ctx context.Context, query string, dest ...any) error {
	rows, err := c.QueryMaps(ctx, query)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no rows")
	}
	// Support SELECT 1 and metric scans by positional order of map values is unstable.
	// Callers should use typed helpers instead for multi-column rows.
	if len(dest) == 1 {
		switch d := dest[0].(type) {
		case *uint8:
			*d = uint8(asFloat(firstValue(rows[0])))
			return nil
		case *uint64:
			*d = uint64(asFloat(firstValue(rows[0])))
			return nil
		case *float64:
			*d = asFloat(firstValue(rows[0]))
			return nil
		case *string:
			*d = fmt.Sprint(firstValue(rows[0]))
			return nil
		}
	}
	return fmt.Errorf("unsupported QueryRow dest")
}

func firstValue(m map[string]any) any {
	for _, v := range m {
		return v
	}
	return nil
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	case bool:
		if t {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func quoteTime(t time.Time) string {
	return "'" + t.UTC().Format("2006-01-02 15:04:05") + "'"
}

func quoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}
