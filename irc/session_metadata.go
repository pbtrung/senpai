package irc

import (
	"strings"
)

func (s *Session) handleMetadataMessage(msg Message) (Event, error) {
	switch msg.Command {
	case "MARKREAD":
		if len(msg.Params) < 2 {
			break
		}
		var target, timestamp string
		if err := msg.ParseParams(&target, &timestamp); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(timestamp, "timestamp=") {
			return nil, nil
		}
		timestamp = strings.TrimPrefix(timestamp, "timestamp=")
		t, ok := parseTimestamp(timestamp)
		if !ok {
			return nil, nil
		}

		channelCf := s.Casemap(target)
		if c, ok := s.channels[channelCf]; ok {
			c.Read = t
			s.channels[channelCf] = c
			if !c.complete {
				return nil, nil
			}
		}

		return ReadEvent{
			Target:    target,
			Timestamp: t,
		}, nil

	case "METADATA":
		// METADATA <Target> <Key> <Visibility> <Value>
		if len(msg.Params) < 4 {
			break
		}
		var target, key, value string
		if err := msg.ParseParams(&target, &key, nil, &value); err != nil {
			return nil, err
		}
		targetCf := s.Casemap(target)
		m := s.metadata[targetCf]
		switch key {
		case "soju.im/pinned":
			m.Pinned = value == "1"
		case "soju.im/muted":
			m.Muted = value == "1"
		}
		s.metadata[targetCf] = m
		ev := MetadataChangeEvent{
			Target: target,
			Pinned: m.Pinned,
			Muted:  m.Muted,
		}
		return ev, nil

	case "BOUNCER":
		if len(msg.Params) < 3 {
			break
		}
		if msg.Params[0] != "NETWORK" || s.netID != "" {
			break
		}
		id := msg.Params[1]
		event := BouncerNetworkEvent{
			ID: id,
		}
		if msg.Params[2] != "*" {
			event.Attrs = parseTags(msg.Params[2])
		} else {
			event.Delete = true
		}
		return event, nil
	}
	return nil, nil
}
