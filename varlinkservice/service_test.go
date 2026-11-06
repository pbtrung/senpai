package varlinkservice

import (
	"errors"
	"net"
	"testing"
	"time"

	govarlink "github.com/emersion/go-varlink"
)

type testBackend struct{ links chan string }

func (b testBackend) OpenLink(in *OpenLinkIn) (*OpenLinkOut, error) {
	if in.Link == "fail" {
		return nil, &govarlink.ServerError{Name: "org.varlink.service.InvalidParameter", Parameters: map[string]string{"parameter": "link"}}
	}
	b.links <- in.Link
	return &OpenLinkOut{}, nil
}

func TestOpenLinkWireProtocol(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	backend := testBackend{links: make(chan string, 1)}
	server := &govarlink.Server{Handler: Handler{Backend: backend}}
	go server.Serve(l)
	conn, err := net.DialTimeout("tcp", l.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	client := Client{govarlink.NewClient(conn)}
	defer client.Close()
	if _, err := client.OpenLink(&OpenLinkIn{Link: "irc://example.org/#senpai"}); err != nil {
		t.Fatal(err)
	}
	if got := <-backend.links; got != "irc://example.org/#senpai" {
		t.Fatalf("link changed: %q", got)
	}
	_, err = client.OpenLink(&OpenLinkIn{Link: "fail"})
	var remote *govarlink.ClientError
	if !errors.As(err, &remote) || remote.Name != "org.varlink.service.InvalidParameter" {
		t.Fatalf("backend error changed: %v", err)
	}
	err = client.Do("fr.delthas.senpai.Unknown", struct{}{}, &struct{}{})
	if !errors.As(err, &remote) || remote.Name != "org.varlink.service.MethodNotFound" {
		t.Fatalf("unknown method error changed: %v", err)
	}
}
