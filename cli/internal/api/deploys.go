package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/whisk-run/contract/status"
)

// Terminal reports whether a deploy status is final.
func Terminal(s status.Deploy) bool { return s.Finished() }

func appPath(org, app string) string {
	return "/orgs/" + pathSeg(org) + "/apps/" + pathSeg(app)
}

// ListDeploys returns one page of the app's deploys, newest first.
func (c *Client) ListDeploys(ctx context.Context, org, app string, limit int) ([]Deploy, error) {
	p, err := getPage[Deploy](ctx, c, appPath(org, app)+"/deploys", "", limit)
	return p.Items, err
}

// GetDeploy fetches one deploy.
func (c *Client) GetDeploy(ctx context.Context, org, app, id string) (Deploy, error) {
	var out Deploy
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/deploys/"+pathSeg(id), nil, &out)
}

// CreateDeploy queues a redeploy or a rollback; the answer is the new deploy.
func (c *Client) CreateDeploy(ctx context.Context, org, app string, req CreateDeployRequest) (Deploy, error) {
	var out Deploy
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/deploys", req, &out)
}

// CancelDeploy cancels a deploy that has not finished.
func (c *Client) CancelDeploy(ctx context.Context, org, app, id string) (Deploy, error) {
	var out Deploy
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/deploys/"+pathSeg(id)+"/cancel", map[string]any{}, &out)
}

// DeployEvents streams the deploy's events, calling fn for each until the stream ends. done is
// true when the last event carried Done. A stream that ends early returns done false and no
// error; one that sends nothing, not even a keepalive, for 45 s is dropped as *Unavailable.
// The caller decides whether to reconnect.
func (c *Client) DeployEvents(ctx context.Context, org, app, id string, fn func(DeployEvent) error) (done bool, err error) {
	err = c.stream(ctx, appPath(org, app)+"/deploys/"+pathSeg(id)+"/events", func(r io.Reader) error {
		var rerr error
		done, rerr = readSSE(r, fn)
		return rerr
	})
	return done, err
}

// readSSE parses server-sent events: data: lines joined by newlines, an empty line ends one.
func readSSE(r io.Reader, fn func(DeployEvent) error) (bool, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var data []string
	flush := func() (bool, error) {
		if len(data) == 0 {
			return false, nil
		}
		raw := strings.Join(data, "\n")
		data = nil
		var ev DeployEvent
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			return false, nil
		}
		if err := fn(ev); err != nil {
			return ev.Done, err
		}
		return ev.Done, nil
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "":
			if done, err := flush(); done || err != nil {
				return done, err
			}
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if done, err := flush(); done || err != nil {
		return done, err
	}
	if err := sc.Err(); err != nil {
		return false, &Unavailable{Err: err}
	}
	return false, nil
}

// BuildLog fetches a build's log as text. While the build runs the server keeps the
// connection open and follows the log; the context bounds the wait.
func (c *Client) BuildLog(ctx context.Context, org, app, buildID string) (string, error) {
	resp, err := c.open(ctx, appPath(org, app)+"/builds/"+pathSeg(buildID)+"/log", "text/plain")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return string(raw), &Unavailable{Err: err}
	}
	return string(raw), nil
}
