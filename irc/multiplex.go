package irc

const NetworkTag = "soju.im/network"

// NewVirtualSession attaches a network to a multiplex-capable bouncer session.
// Callers must route incoming messages by NetworkTag before handling batches.
// Like ordinary sessions, these methods are called from the application's event loop.
func (s *Session) NewVirtualSession(id string) *Session {
	if s.closed || s.netID != "" || id == "" || !s.HasCapability("soju.im/multiplex") {
		return nil
	}
	v := newSession(s.out, SessionParams{
		Nickname: s.nick, Username: s.user, RealName: s.real, NetID: id,
	})
	v.virtual = true
	v.parent = s
	v.registered = true
	v.acct = s.acct
	v.host = s.host
	for name, value := range s.availableCaps {
		v.availableCaps[name] = value
	}
	for name := range s.enabledCaps {
		if name != "soju.im/multiplex" && name != "soju.im/bouncer-networks-notify" {
			v.enabledCaps[name] = struct{}{}
		}
	}
	// The writer tracks labeled-response independently for each virtual network.
	if v.HasCapability("labeled-response") {
		v.send(Message{Command: labelEnableCommand})
	} else {
		v.send(Message{Command: labelDisableCommand})
	}
	s.send(NewMessage("BOUNCER", "ATTACH", id))
	if v.HasCapability("draft/metadata-2") {
		v.send(NewMessage("METADATA", "*", "SUB", "soju.im/pinned", "soju.im/muted"))
	}
	// Deferred capabilities may still be awaiting ACK on the parent when its
	// network list arrives. Virtual connections negotiate those independently.
	for name := range v.availableCaps {
		if _, supported := SupportedCapabilities[name]; supported && !v.HasCapability(name) &&
			name != "sasl" && name != "soju.im/multiplex" && name != "soju.im/bouncer-networks-notify" {
			// Upstream-specific capabilities can disappear during attachment.
			// Request separately so one stale capability cannot reject the rest.
			v.send(NewMessage("CAP", "REQ", name))
		}
	}
	return v
}

func (s *Session) IsVirtual() bool { return s.virtual }

func (s *Session) Detach(id string) {
	s.send(NewMessage("BOUNCER", "DETACH", id))
}

func (s *Session) send(msg Message) {
	if s.closed || (s.parent != nil && s.parent.closed) {
		return
	}
	if s.virtual {
		msg = msg.WithTag(NetworkTag, s.netID)
	}
	s.out <- msg
}
