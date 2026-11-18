package irc

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

// special internal Message commands to propagate labeled response status to the writing goroutine
var labelEnableCommand = ":enable_labeled_response"
var labelDisableCommand = ":disable_labeled_response"

const chanCapacity = 64

func ChanInOut(conn net.Conn) (in <-chan Message, out chan<- Message) {
	in_ := make(chan Message, chanCapacity)
	out_ := make(chan Message, chanCapacity)

	const keepAlive = 30 * time.Second
	const maxRTT = 10 * time.Second
	var last atomic.Value
	last.Store(time.Now())
	// A peer which never sends its first line must time out as well.
	conn.SetReadDeadline(time.Now().Add(keepAlive + maxRTT))

	go func() {
		r := bufio.NewScanner(conn)
		// Account for negotiated 64 KiB lines plus IRCv3 message tags.
		r.Buffer(make([]byte, 4096), 128*1024)
		for r.Scan() {
			line := r.Text()
			line = strings.ToValidUTF8(line, string([]rune{unicode.ReplacementChar}))
			// Workaround for https://bugs.unrealircd.org/view.php?id=6598
			line = strings.TrimSuffix(line, "\r")
			msg, err := ParseMessage(line)
			if err != nil {
				continue
			}
			now := time.Now()
			last.Store(now)
			conn.SetReadDeadline(now.Add(keepAlive + maxRTT))
			in_ <- msg
		}
		close(in_)
	}()

	go func() {
		defer func() {
			conn.Close()
			// Unblock senders until the owner observes the closed input and
			// closes output. Otherwise a write error can strand the event loop
			// on a full output queue before it can process the disconnect.
			for range out_ {
			}
		}()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		labelOff := 1
		labeledResponse := make(map[string]bool)
	outer:
		for {
			select {
			case msg, ok := <-out_:
				if !ok {
					break outer
				}
				network := msg.Tags[NetworkTag]
				if msg.Command == labelEnableCommand {
					labeledResponse[network] = true
					continue
				}
				if msg.Command == labelDisableCommand {
					delete(labeledResponse, network)
					continue
				}
				if labeledResponse[network] && msg.Tags["batch"] == "" &&
					!(msg.Command == "BATCH" && len(msg.Params) > 0 && strings.HasPrefix(msg.Params[0], "-")) {
					label := strconv.Itoa(labelOff)
					labelOff++
					if msg.Tags == nil {
						msg.Tags = map[string]string{"label": label}
					} else {
						msg.Tags["label"] = label
					}
				}

				last.Store(time.Now())
				// TODO send messages by batches
				conn.SetWriteDeadline(time.Now().Add(maxRTT))
				_, err := fmt.Fprintf(conn, "%s\r\n", msg.String())
				if err != nil {
					break outer
				}
			case <-t.C:
				now := time.Now()
				if last.Load().(time.Time).Add(keepAlive).After(now) {
					continue
				}
				if last.Load().(time.Time).Add(keepAlive + maxRTT).Before(now) {
					// probably out of sleep, reset connection
					conn.Close()
					continue
				}
				last.Store(now)
				conn.SetWriteDeadline(now.Add(maxRTT))
				_, err := fmt.Fprint(conn, "PING _\r\n")
				if err != nil {
					break outer
				}
			}
		}
	}()

	return in_, out_
}
