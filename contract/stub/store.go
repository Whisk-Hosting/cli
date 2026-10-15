package stub

import (
	"sort"
	"sync"
	"time"
)

// Stored state of one stub run: webhook events, approvals, the email the app sent, custom
// domains and the files an inbox kept. Memory only; a restart is a
// fresh platform, which is what a local run wants.

type webhookEvent struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	ReceivedAt time.Time         `json:"received_at"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"-"`
	BodyText   string            `json:"body"`
	Verified   bool              `json:"verified"`
	Reason     string            `json:"reason,omitempty"`
	// DeliveryStatus is queued, delivered, failed or dead; never_delivered for unverified, and
	// skipped for an inbox message that was dropped (Reason says why).
	DeliveryStatus string `json:"delivery_status"`
	Attempts       int    `json:"attempts"`
	LastStatus     int    `json:"last_status,omitempty"`
}

type approval struct {
	ID          string         `json:"id"`
	To          string         `json:"to"`
	Title       string         `json:"title"`
	Data        any            `json:"data"`
	Function    string         `json:"function,omitempty"`
	RunID       string         `json:"run_id,omitempty"`
	RequestedAt time.Time      `json:"requested_at"`
	Status      string         `json:"status"` // pending | approved | rejected
	Decision    map[string]any `json:"decision,omitempty"`
}

type store struct {
	mu        sync.Mutex
	events    map[string]*webhookEvent
	approvals map[string]*approval
	mails     []*sentMail // oldest first, at most maxSent
	domains   *domains
	files     map[string]*stubFile
}

func newStore() *store {
	return &store{events: map[string]*webhookEvent{}, approvals: map[string]*approval{}, domains: newDomains(), files: map[string]*stubFile{}}
}

func (s *store) putMail(m *sentMail) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mails = append(s.mails, m)
	if len(s.mails) > maxSent {
		s.mails = s.mails[len(s.mails)-maxSent:]
	}
}

// mailList is what the stub took, newest first.
func (s *store) mailList() []*sentMail {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*sentMail, 0, len(s.mails))
	for i := len(s.mails) - 1; i >= 0; i-- {
		out = append(out, s.mails[i])
	}
	return out
}

func (s *store) putFile(f *stubFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[f.ID] = f
}

func (s *store) file(id string) (*stubFile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	return f, ok
}

func (s *store) putEvent(e *webhookEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[e.ID] = e
}

func (s *store) event(id string) (*webhookEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[id]
	return e, ok
}

func (s *store) update(id string, f func(*webhookEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.events[id]; ok {
		f(e)
	}
}

func (s *store) eventsFor(source string) []webhookEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []webhookEvent
	for _, e := range s.events {
		if e.Source == source {
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.Before(out[j].ReceivedAt) })
	return out
}

func (s *store) putApproval(a *approval) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.approvals[a.ID]; !exists {
		s.approvals[a.ID] = a
	}
}

func (s *store) approval(id string) (*approval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	return a, ok
}

func (s *store) decide(id string, decision map[string]any) (*approval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok || a.Status != "pending" {
		return a, false
	}
	a.Status, _ = decision["decision"].(string)
	a.Decision = decision
	return a, true
}

func (s *store) approvalList(status string) []approval {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []approval
	for _, a := range s.approvals {
		if status == "" || a.Status == status {
			out = append(out, *a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.Before(out[j].RequestedAt) })
	return out
}
