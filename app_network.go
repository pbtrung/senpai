package senpai

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/events"
	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
	"git.sr.ht/~delthas/senpai/varlinkservice"
	"golang.org/x/net/proxy"
)

func (app *App) wantsNetwork(netID string) bool {
	if app.win.ShouldExit() {
		return false
	}
	app.networkLock.RLock()
	_, ok := app.networks[netID]
	app.networkLock.RUnlock()
	return ok
}

func (app *App) findNetworkByLink(link string) (host string, target string, netID string) {
	// links often do not contain encoded #
	link = strings.ReplaceAll(link, "#", "%23")

	u, err := url.Parse(link)
	if err != nil {
		return "", "", ""
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		port = "6697"
	}
	target = strings.TrimLeft(u.Path, "/")
	if i := strings.IndexAny(target, "/,"); i >= 0 {
		target = target[:i]
	}
	app.networkLock.Lock()
	defer app.networkLock.Unlock()
	for netID, attrs := range app.networks {
		if attrs["host"] != host {
			continue
		}
		p := attrs["port"]
		if p == "" {
			p = "6697"
		}
		if p != port {
			continue
		}
		return u.Host, target, netID
	}
	return u.Host, target, ""
}

// ircLoop maintains a connection to the IRC server by connecting and then
// forwarding IRC events to app.events repeatedly.
func (app *App) ircLoop(netID string) {
	var auth irc.SASLClient
	if app.cfg.Password != nil {
		auth = &irc.SASLPlain{
			Username: app.cfg.User,
			Password: *app.cfg.Password,
		}
	}
	params := irc.SessionParams{
		Nickname: app.cfg.Nick,
		Username: app.cfg.User,
		RealName: app.cfg.Real,
		NetID:    netID,
		Auth:     auth,
	}
	const throttleInterval = 6 * time.Second
	const throttleMax = 1 * time.Minute
	var delay time.Duration = 0
	for app.wantsNetwork(netID) {
		time.Sleep(delay)
		if !app.wantsNetwork(netID) {
			break
		}
		if delay < throttleMax {
			delay += throttleInterval
		}
		conn := app.connect(netID)
		if conn == nil {
			continue
		}
		if !app.wantsNetwork(netID) {
			conn.Close()
			break
		}
		delay = throttleInterval

		in, out := irc.ChanInOut(conn)
		if app.cfg.Debug {
			out = app.debugOutputMessages(netID, out)
		}
		session := irc.NewSession(out, params)
		if !app.postEvent(event{
			src: netID, content: session,
		}) {
			session.Close()
			conn.Close()
			return
		}
		go func() {
			for stop := range session.TypingStops() {
				app.postEvent(event{
					src:     netID,
					content: stop,
				})
			}
		}()
		for msg := range in {
			if app.cfg.Debug {
				app.queueStatusLine(netID, ui.Line{
					At:   time.Now(),
					Head: ui.PlainString("IN --"),
					Body: ui.PlainString(msg.String()),
				})
			}
			app.postEvent(event{
				src:     messageNetwork(netID, msg),
				content: msg,
			})
		}
		app.postEvent(event{
			src:     netID,
			content: nil,
		})
		app.queueStatusLine(netID, ui.Line{
			Head: ui.ColorString("!!", ui.ColorRed),
			Body: ui.PlainString("Connection lost"),
		})
	}
}

func (app *App) connect(netID string) net.Conn {
	app.queueStatusLine(netID, ui.Line{
		Head: ui.PlainString("--"),
		Body: ui.PlainSprintf("Connecting to %s...", app.cfg.Addr),
	})
	conn, err := app.tryConnect()
	if err == nil {
		return conn
	}
	app.queueStatusLine(netID, ui.Line{
		Head: ui.ColorString("!!", ui.ColorRed),
		Body: ui.PlainSprintf("Connection failed: %v", err),
	})
	return nil
}

func (app *App) tryConnect() (conn net.Conn, err error) {
	addr := app.cfg.Addr
	colonIdx := strings.LastIndexByte(addr, ':')
	bracketIdx := strings.LastIndexByte(addr, ']')
	if colonIdx <= bracketIdx {
		// either colonIdx < 0, or the last colon is before a ']' (end
		// of IPv6 address). -> missing port
		if app.cfg.TLS {
			addr += ":6697"
		} else {
			addr += ":6667"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		LocalAddr: app.cfg.LocalAddr,
	}
	conn, err = proxy.FromEnvironmentUsing(dialer).(proxy.ContextDialer).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect: %v", err)
	}

	if app.cfg.TLS {
		host, _, _ := net.SplitHostPort(addr) // should succeed since net.Dial did.
		conn = tls.Client(conn, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: app.cfg.TLSSkipVerify,
			NextProtos:         []string{"irc"},
		})
		err = conn.(*tls.Conn).HandshakeContext(ctx)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("tls handshake: %v", err)
		}
	}

	return
}

func (app *App) debugOutputMessages(netID string, out chan<- irc.Message) chan<- irc.Message {
	debugOut := make(chan irc.Message, cap(out))
	go func() {
		for msg := range debugOut {
			const placeholder = "<removed>"
			d := msg
			if msg.Command == "PASS" && len(d.Params) >= 1 {
				d.Params = append([]string{placeholder}, d.Params[1:]...)
			} else if msg.Command == "OPER" && len(d.Params) >= 2 {
				d.Params = append([]string{d.Params[0], placeholder}, d.Params[2:]...)
			} else if msg.Command == "AUTHENTICATE" && len(d.Params) >= 1 {
				switch d.Params[0] {
				case "*", "PLAIN":
				default:
					d.Params = append([]string{placeholder}, d.Params[1:]...)
				}
			}
			app.queueStatusLine(netID, ui.Line{
				At:   time.Now(),
				Head: ui.PlainString("OUT --"),
				Body: ui.PlainString(d.String()),
			})
			out <- msg
		}
		close(out)
	}()
	return debugOut
}

func messageNetwork(netID string, msg irc.Message) string {
	if netID == "" && msg.Tags[irc.NetworkTag] != "" {
		return msg.Tags[irc.NetworkTag]
	}
	return netID
}

func (app *App) startNetwork(id string) {
	if root := app.sessions[""]; root != nil && root.HasCapability("soju.im/multiplex") {
		if session := root.NewVirtualSession(id); session != nil {
			app.handleIRCEvent(id, session)
			go app.forwardTyping(id, session)
		}
		return
	}
	go app.ircLoop(id)
}

func (app *App) forwardTyping(id string, session *irc.Session) {
	for stop := range session.TypingStops() {
		app.postEvent(event{src: id, content: stop})
	}
}

func (app *App) OpenLink(in *varlinkservice.OpenLinkIn) (*varlinkservice.OpenLinkOut, error) {
	app.postEvent(event{
		src: "*",
		content: &events.EventOpenLink{
			Link: in.Link,
		},
	})
	return nil, nil
}
