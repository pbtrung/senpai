package irc

import (
	"fmt"
	"strconv"
	"time"
)

type HistoryRequest struct {
	s              *Session
	target         string
	command        string
	bounds         []string
	limit          int
	requestedLimit int
}

func formatTimestamp(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("timestamp=%04d-%02d-%02dT%02d:%02d:%02d.%03dZ",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1e6)
}

func (r *HistoryRequest) WithLimit(limit int) *HistoryRequest {
	r.requestedLimit = limit
	if limit < r.s.historyLimit {
		r.limit = limit
	} else {
		r.limit = r.s.historyLimit
	}
	return r
}

func (r *HistoryRequest) doRequest() {
	if !r.s.HasCapability("draft/chathistory") {
		return
	}

	targetCf := r.s.casemap(r.target)
	if _, ok := r.s.chReqs[targetCf]; ok {
		return
	}
	r.s.chReqs[targetCf] = struct{}{}

	args := make([]string, 0, len(r.bounds)+3)
	args = append(args, r.command)
	if r.target != "" {
		args = append(args, r.target)
	}
	args = append(args, r.bounds...)
	args = append(args, strconv.Itoa(r.limit))
	r.s.send(NewMessage("CHATHISTORY", args...))
}

func (r *HistoryRequest) After(t time.Time) {
	r.command = "AFTER"
	r.bounds = []string{formatTimestamp(t)}
	r.doRequest()
}

func (r *HistoryRequest) Before(t time.Time) {
	r.command = "BEFORE"
	r.bounds = []string{formatTimestamp(t)}
	r.doRequest()
}

func (r *HistoryRequest) Latest() {
	r.command = "LATEST"
	r.bounds = []string{"*"}
	r.doRequest()
}

func (r *HistoryRequest) Targets(start time.Time, end time.Time) {
	r.command = "TARGETS"
	r.bounds = []string{formatTimestamp(start), formatTimestamp(end)}
	r.target = ""
	if r.s.HasCapability("soju.im/unread") && r.requestedLimit > 0 {
		r.limit = r.requestedLimit
	}
	r.doRequest()
}

func (s *Session) NewHistoryRequest(target string) *HistoryRequest {
	return &HistoryRequest{
		s:      s,
		target: target,
		limit:  s.historyLimit,
	}
}
