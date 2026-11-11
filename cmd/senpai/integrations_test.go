package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sr.ht/~delthas/senpai/varlinkservice"
	"github.com/emersion/go-varlink"
)

type linkTestBackend struct{ links chan string }

func (b linkTestBackend) OpenLink(in *varlinkservice.OpenLinkIn) (*varlinkservice.OpenLinkOut, error) {
	b.links <- in.Link
	return &varlinkservice.OpenLinkOut{}, nil
}

func TestOpenLinkSkipsStaleSockets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0-stale.sock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "1-live.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	backend := linkTestBackend{links: make(chan string, 1)}
	server := varlink.NewServer()
	server.Handler = varlinkservice.Handler{Backend: backend}
	go server.Serve(listener)
	const link = "ircs://example.com/test"
	ok, err := sendOpenLink(dir, link)
	if err != nil || !ok {
		t.Fatalf("live instance not reached: %v, %v", ok, err)
	}
	select {
	case got := <-backend.links:
		if got != link {
			t.Fatalf("wrong link: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("live instance did not receive link")
	}
}
