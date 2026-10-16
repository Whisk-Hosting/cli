package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"strconv"
	"strings"
	"sync"

	"github.com/whisk-run/contract/run"
)

// Meter limits (BROKER.md §2 step 9).
const (
	// MaxMetered is the most of an answer a meter reads; past it the meter counts what it found.
	MaxMetered = 64 << 20
	// MaxEventLine is the longest line of an event stream a meter reads.
	MaxEventLine = 1 << 20
)

// Meter reads a copy of an AI connection's answer as it streams and finds the number at each of
// the connection's cost parts (BROKER.md §2 step 9). An event stream is read event by event (each
// data: line holding JSON) and anything else as JSON documents token by token, so a large answer
// is never held whole; the elements of a top-level array count as events. For each part the last
// value seen counts, and within one event the part's first pointer found. Write never fails and
// never changes what the app receives; Close waits for the reading to finish.
type Meter struct {
	targets map[string][]meterTarget
	sse     bool

	mu     sync.Mutex
	values map[int]float64
	read   int64
	closed bool

	line     []byte
	overlong bool

	pw   *io.PipeWriter
	done chan struct{}
}

// NewMeter is a meter for an answer of the content type.
func NewMeter(a AI, contentType string) *Meter {
	m := &Meter{targets: targets(a), values: map[int]float64{}}
	if mt, _, err := mime.ParseMediaType(contentType); err == nil && mt == "text/event-stream" {
		m.sse = true
		return m
	}
	pr, pw := io.Pipe()
	m.pw, m.done = pw, make(chan struct{})
	run.Spawn(context.Background(), "meter an answer", func(context.Context) error {
		defer close(m.done)
		m.readJSON(pr)
		_, err := io.Copy(io.Discard, pr)
		return err
	})
	return m
}

// Write feeds the meter a copy of the next bytes of the answer.
func (m *Meter) Write(p []byte) (int, error) {
	n := len(p)
	if m.closed || m.read >= MaxMetered {
		return n, nil
	}
	if left := MaxMetered - m.read; int64(len(p)) > left {
		p = p[:left]
	}
	m.read += int64(len(p))
	if m.sse {
		m.feedLines(p)
		return n, nil
	}
	_, _ = m.pw.Write(p)
	if m.read >= MaxMetered {
		_ = m.pw.Close()
	}
	return n, nil
}

// Close ends the answer and answers the number found for each part, by index.
func (m *Meter) Close() map[int]float64 {
	if !m.closed {
		m.closed = true
		if m.sse {
			if len(m.line) > 0 && !m.overlong {
				m.event(m.line)
			}
		} else {
			_ = m.pw.Close()
			<-m.done
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int]float64, len(m.values))
	for k, v := range m.values {
		out[k] = v
	}
	return out
}

func (m *Meter) feedLines(p []byte) {
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		chunk := p
		if i >= 0 {
			chunk = p[:i]
		}
		if !m.overlong {
			if len(m.line)+len(chunk) > MaxEventLine {
				m.overlong, m.line = true, nil
			} else {
				m.line = append(m.line, chunk...)
			}
		}
		if i < 0 {
			return
		}
		if !m.overlong {
			m.event(m.line)
		}
		m.line, m.overlong = m.line[:0], false
		p = p[i+1:]
	}
}

// event reads one line of an event stream: a data: line holding a JSON document is one event.
func (m *Meter) event(line []byte) {
	data, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte("data:"))
	if !ok {
		return
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || (data[0] != '{' && data[0] != '[') {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if dec.Decode(&doc) != nil {
		return
	}
	found := map[int]rankedValue{}
	m.walk(doc, nil, found)
	m.merge(found)
}

type rankedValue struct {
	rank  int
	value float64
}

func (m *Meter) walk(v any, path []string, found map[int]rankedValue) {
	switch d := v.(type) {
	case map[string]any:
		for k, x := range d {
			m.walk(x, append(path, k), found)
		}
	case []any:
		for i, x := range d {
			m.walk(x, append(path, strconv.Itoa(i)), found)
		}
	default:
		m.see(path, v, found)
	}
}

func (m *Meter) see(path []string, v any, found map[int]rankedValue) {
	ts, ok := m.targets[strings.Join(path, "\x00")]
	if !ok {
		return
	}
	n, ok := numberOf(v)
	if !ok {
		return
	}
	for _, t := range ts {
		if have, ok := found[t.part]; !ok || t.rank < have.rank {
			found[t.part] = rankedValue{t.rank, n}
		}
	}
}

func (m *Meter) merge(found map[int]rankedValue) {
	if len(found) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for part, rv := range found {
		m.values[part] = rv.value
	}
}

// frame is one open object or array while reading JSON token by token.
type frame struct {
	object  bool
	key     string
	index   int
	wantKey bool
}

// readJSON reads JSON documents token by token. Each top-level document is an event, or each
// element of a top-level array is.
func (m *Meter) readJSON(r io.Reader) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var stack []frame
	found := map[int]rankedValue{}
	path := func() []string {
		out := make([]string, 0, len(stack))
		for i, f := range stack {
			if i == 0 && !f.object {
				continue // a top-level array's elements are events
			}
			if f.object {
				out = append(out, f.key)
			} else {
				out = append(out, strconv.Itoa(f.index))
			}
		}
		return out
	}
	// ended marks a value finished in the innermost open container, ending an event when it
	// was a top-level document or an element of a top-level array.
	ended := func() {
		switch {
		case len(stack) == 0, len(stack) == 1 && !stack[0].object:
			m.merge(found)
			found = map[int]rankedValue{}
		}
		if len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.object {
				top.wantKey = true
			} else {
				top.index++
			}
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				stack = append(stack, frame{object: d == '{', wantKey: d == '{'})
			default:
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				ended()
			}
			continue
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
			stack[n-1].key, _ = tok.(string)
			stack[n-1].wantKey = false
			continue
		}
		m.see(path(), tok, found)
		ended()
	}
}
