package irc

import (
	"fmt"
	"time"
)

func (s *Session) handleRegistered(msg Message) (Event, error) {
	if handled, assembled, err := s.handleMultiline(msg); handled {
		if err != nil || assembled == nil {
			return nil, err
		}
		return s.handleRegistered(*assembled)
	}
	if id, ok := msg.Tags["batch"]; ok && id != "" {
		if id == s.targetsBatchID {
			var target, timestamp string
			if err := msg.ParseParams(nil, &target, &timestamp); err != nil {
				return nil, err
			}
			t, ok := parseTimestamp(timestamp)
			if !ok {
				return nil, nil
			}
			s.targetsBatch.Targets[target] = t
			_, unread := msg.Tags["soju.im/unread"]
			_, mention := msg.Tags["soju.im/mention"]
			s.targetsBatch.Unread[target] = TargetUnread{Unread: unread, Mention: mention}
			return nil, nil
		} else if id == s.searchBatchID {
			ev, err := s.handleMessageRegistered(msg, true)
			if err != nil {
				return nil, err
			}
			if ev, ok := ev.(MessageEvent); ok {
				s.searchBatch.Messages = append(s.searchBatch.Messages, ev)
				return nil, nil
			}
		} else if id == s.bouncerNetworksBatchID {
			ev, err := s.handleMessageRegistered(msg, true)
			if err != nil {
				return nil, err
			}
			if ev, ok := ev.(BouncerNetworkEvent); ok {
				s.bouncerNetworksBatch = append(s.bouncerNetworksBatch, ev)
				return nil, nil
			}
		} else if b, ok := s.chBatches[id]; ok {
			ev, err := s.handleMessageRegistered(msg, true)
			if err != nil {
				return nil, err
			}
			if ev != nil {
				s.chBatches[id] = HistoryEvent{
					Target:   b.Target,
					End:      b.End,
					Messages: append(b.Messages, ev),
				}
			}
			return nil, nil
		}
	}
	return s.handleMessageRegistered(msg, false)
}

func (s *Session) handleBatchMessage(msg Message) (Event, error) {
	switch msg.Command {
	case "BATCH":
		var id string
		if err := msg.ParseParams(&id); err != nil {
			return nil, err
		}
		if len(id) < 2 || (id[0] != '+' && id[0] != '-') {
			return nil, fmt.Errorf("invalid batch id: %q", id)
		}

		batchStart := id[0] == '+'
		id = id[1:]

		if batchStart {
			var name string
			if err := msg.ParseParams(nil, &name); err != nil {
				return nil, err
			}

			switch name {
			case "chathistory":
				var target string
				if err := msg.ParseParams(nil, nil, &target); err != nil {
					return nil, err
				}

				_, end := msg.Tags["draft/chathistory-end"]
				s.chBatches[id] = HistoryEvent{Target: target, End: end}
			case "draft/chathistory-targets":
				s.targetsBatchID = id
				s.targetsBatch = HistoryTargetsEvent{Targets: make(map[string]time.Time), Unread: make(map[string]TargetUnread)}
			case "soju.im/search":
				s.searchBatchID = id
				s.searchBatch = SearchEvent{}
			case "soju.im/bouncer-networks":
				s.bouncerNetworksBatchID = id
				s.bouncerNetworksBatch = BouncerNetworkListEvent{}
			}
		} else {
			if b, ok := s.chBatches[id]; ok {
				delete(s.chBatches, id)
				delete(s.chReqs, s.Casemap(b.Target))
				return b, nil
			} else if s.targetsBatchID == id {
				s.targetsBatchID = ""
				delete(s.chReqs, "")
				return s.targetsBatch, nil
			} else if s.searchBatchID == id {
				s.searchBatchID = ""
				return s.searchBatch, nil
			} else if s.bouncerNetworksBatchID == id {
				s.bouncerNetworksBatchID = ""
				return s.bouncerNetworksBatch, nil
			}
		}
	}
	return nil, nil
}
