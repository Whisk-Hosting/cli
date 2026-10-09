package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PlatformError is one group in Whisk's own error log (CONTROL-PLANE.md §6.31): one problem the
// platform's services logged, or a crash in people's browsers, with how often and when.
type PlatformError struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Source     string     `json:"source"`
	Status     string     `json:"status"`
	Count      int64      `json:"count"`
	FirstSeen  time.Time  `json:"first_seen"`
	LastSeen   time.Time  `json:"last_seen"`
	Host       string     `json:"host,omitempty"`
	Sample     string     `json:"sample"`
	Note       string     `json:"note,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Returned   int        `json:"returned"`
}

// PlatformErrorPage is a page of the error log.
type PlatformErrorPage struct {
	Items      []PlatformError `json:"items"`
	NextCursor string          `json:"next_cursor"`
	Open       int             `json:"open"`
	ScannedTo  time.Time       `json:"scanned_to"`
}

// PlatformErrorQuery filters the error log.
type PlatformErrorQuery struct {
	Status string
	Source string
	Cursor string
	Limit  int
}

// PlatformErrors reads a page of the error log, open groups unless asked.
func (c *Client) PlatformErrors(ctx context.Context, q PlatformErrorQuery) (PlatformErrorPage, error) {
	v := url.Values{}
	for k, val := range map[string]string{"status": q.Status, "source": q.Source, "cursor": q.Cursor} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/operator/errors"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out PlatformErrorPage
	err := c.Do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// PlatformError reads one group.
func (c *Client) PlatformError(ctx context.Context, id string) (PlatformError, error) {
	var out PlatformError
	err := c.Do(ctx, http.MethodGet, "/operator/errors/"+pathSeg(id), nil, &out)
	return out, err
}

// SetPlatformErrorStatus resolves, ignores or reopens a group, with a note saying what was done.
func (c *Client) SetPlatformErrorStatus(ctx context.Context, id, status, note string) (PlatformError, error) {
	body := map[string]string{"status": status}
	if note != "" {
		body["note"] = note
	}
	var out PlatformError
	err := c.Do(ctx, http.MethodPost, "/operator/errors/"+pathSeg(id), body, &out)
	return out, err
}
