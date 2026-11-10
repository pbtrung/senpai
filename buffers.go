package senpai

import (
	"slices"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
)

// BufferState remembers a conversation, without storing its messages locally.
type BufferState struct {
	Network  string    `json:"network"`
	Name     string    `json:"name"`
	Read     time.Time `json:"read,omitzero"`
	ClosedAt time.Time `json:"closed_at,omitzero"`
}

type networkBuffers struct {
	casemap func(string) string
	buffers map[string]BufferState
}

type bufferStore map[string]*networkBuffers

func (store *bufferStore) network(id string) *networkBuffers {
	if *store == nil {
		*store = make(bufferStore)
	}
	net := (*store)[id]
	if net == nil {
		// Preserve distinct ASCII targets until ISUPPORT tells us otherwise.
		net = &networkBuffers{casemap: irc.CasemapASCII, buffers: make(map[string]BufferState)}
		(*store)[id] = net
	}
	return net
}

func (store *bufferStore) get(network, name string) BufferState {
	net := store.network(network)
	return net.buffers[net.casemap(name)]
}

func (store *bufferStore) put(state BufferState) {
	if state.Name == "" || strings.ContainsAny(state.Name, " \r\n\x00,") {
		return
	}
	net := store.network(state.Network)
	key := net.casemap(state.Name)
	if prev := net.buffers[key]; prev.Read.After(state.Read) {
		state.Read = prev.Read
	}
	net.buffers[key] = state
}

func (store *bufferStore) configure(network string, session *irc.Session) {
	net := store.network(network)
	casemap := irc.CasemapRFC1459
	if session.Casemap("[") == "[" {
		casemap = irc.CasemapASCII
	}
	buffers := net.buffers
	net.casemap = casemap
	net.buffers = make(map[string]BufferState, len(buffers))
	for _, state := range buffers {
		key := casemap(state.Name)
		if prev, ok := net.buffers[key]; ok {
			// Prefer an open copy and the newest read marker when names merge.
			if prev.ClosedAt.IsZero() || (!state.ClosedAt.IsZero() && prev.ClosedAt.After(state.ClosedAt)) {
				state.ClosedAt = prev.ClosedAt
			}
		}
		store.put(state)
	}
}

func (store *bufferStore) snapshot() []BufferState {
	var states []BufferState
	for _, net := range *store {
		for _, state := range net.buffers {
			states = append(states, state)
		}
	}
	slices.SortFunc(states, func(a, b BufferState) int {
		if c := strings.Compare(a.Network, b.Network); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return states
}

// RestoreBuffers restores the saved conversation list when its networks connect.
func (app *App) RestoreBuffers(states []BufferState) {
	for _, state := range states {
		app.bufferStore.put(state)
	}
}

// BufferStates returns the conversation list and read markers for the next run.
func (app *App) BufferStates() []BufferState {
	for i := 0; ; i++ {
		network, name, ok := app.win.Buffer(i)
		if !ok {
			break
		}
		if name != "" {
			app.bufferStore.put(BufferState{Network: network, Name: name, Read: app.win.ReadMarker(network, name)})
		}
	}
	return app.bufferStore.snapshot()
}

func (app *App) restoreNetworkBuffers(network string, session *irc.Session) []string {
	app.bufferStore.configure(network, session)
	var channels []string
	for _, state := range app.bufferStore.snapshot() {
		if state.Network != network || !state.ClosedAt.IsZero() {
			continue
		}
		if session.IsChannel(state.Name) {
			// Bouncers restore joined channels themselves.
			if !session.IsBouncer() {
				channels = append(channels, state.Name)
			}
		} else {
			app.addUserBuffer(network, state.Name, time.Time{})
		}
	}
	return channels
}

func (app *App) openBuffer(network, name string) {
	state := app.bufferStore.get(network, name)
	if !state.Read.IsZero() {
		app.win.SetRead(network, name, state.Read)
	}
	state.Network, state.Name = network, name
	state.ClosedAt = time.Time{}
	app.bufferStore.put(state)
	if network == app.lastNetID && name == app.lastBuffer {
		app.win.JumpBufferNetwork(network, name)
		app.win.ScrollToBuffer()
		app.lastNetID, app.lastBuffer = "", ""
	}
}

func (app *App) closeBuffer(network, name string, reason string) {
	if name == "" {
		return
	}
	app.bufferStore.put(BufferState{
		Network: network, Name: name,
		Read: app.win.ReadMarker(network, name), ClosedAt: time.Now().UTC(),
	})
	if session := app.sessions[network]; session != nil {
		if session.IsChannel(name) {
			session.Part(name, reason)
		} else {
			session.MonitorRemove(name)
			delete(app.monitor[network], name)
		}
		delete(app.messageBounds, boundKey{network, session.Casemap(name)})
	}
	app.win.RemoveBuffer(network, name)
}

func (app *App) targetClosed(network, name string, latest time.Time) bool {
	state := app.bufferStore.get(network, name)
	return !state.ClosedAt.IsZero() && !latest.After(state.ClosedAt)
}
