package irc

import (
	"reflect"
	"testing"
)

func TestParseMessageParameterWhitespace(t *testing.T) {
	for _, line := range []string{"PRIVMSG   #c   :hello  world", "  PRIVMSG #c :hello  world"} {
		msg, err := ParseMessage(line)
		if err != nil || !reflect.DeepEqual(msg.Params, []string{"#c", "hello  world"}) {
			t.Fatalf("%q: params %q, error %v", line, msg.Params, err)
		}
	}
	msg, err := ParseMessage("PING token   ")
	if err != nil || !reflect.DeepEqual(msg.Params, []string{"token"}) {
		t.Fatalf("trailing spaces added parameters: %#v, %v", msg, err)
	}
}

func TestReplySeverityShortInput(t *testing.T) {
	for _, reply := range []string{"", "9", "90"} {
		if got := ReplySeverity(reply); got != SeverityNote {
			t.Errorf("%q: %v", reply, got)
		}
	}
}
