package api

import (
	"context"
	"io"
	"net/http"

	"github.com/whisk-run/contract/run/runhttp"
)

// EngineDigestHeader carries the commerce engine's SHA-256 (control-plane api.EngineDigestHeader).
const EngineDigestHeader = "X-Whisk-Sha256"

// Engine is a download of the commerce engine: its body to read, or Current when the one whose
// digest was given is still the one served.
type Engine struct {
	Body    io.ReadCloser
	Digest  string
	Current bool
}

// DownloadEngine asks for the commerce engine for a platform, which whisk dev runs beside a
// shop; have is the digest of the one already kept, "" for none.
func (c *Client) DownloadEngine(ctx context.Context, goos, goarch, have string) (Engine, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/commerce-engine/"+pathSeg(goos)+"/"+pathSeg(goarch), nil)
	if err != nil {
		return Engine{}, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", c.UserAgent)
	if have != "" {
		req.Header.Set("If-None-Match", `"`+have+`"`)
	}
	c.authorize(req, nil)
	streaming := runhttp.StreamClient(streamFirstByte)
	streaming.CheckRedirect = c.HTTP.CheckRedirect
	resp, err := streaming.Do(req)
	if err != nil {
		return Engine{}, &Unavailable{Err: err}
	}
	c.noteCLIVersion(resp)
	switch {
	case resp.StatusCode == http.StatusNotModified:
		resp.Body.Close()
		return Engine{Digest: have, Current: true}, nil
	case resp.StatusCode >= 300:
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		return Engine{}, parseError(resp.StatusCode, raw)
	}
	return Engine{Body: resp.Body, Digest: resp.Header.Get(EngineDigestHeader)}, nil
}
