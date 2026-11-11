package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"git.sr.ht/~delthas/senpai/varlinkservice"
	"github.com/emersion/go-varlink"
)

func sendOpenLink(socketDir string, link string) (ok bool, err error) {
	es, err := os.ReadDir(socketDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range es {
		if filepath.Ext(e.Name()) != ".sock" {
			continue
		}
		conn, err := net.Dial("unix", filepath.Join(socketDir, e.Name()))
		if err != nil {
			return false, err
		}
		c := varlinkservice.Client{Client: varlink.NewClient(conn)}
		defer c.Close()
		if _, err := c.OpenLink(&varlinkservice.OpenLinkIn{Link: link}); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func listenVarlink(socketPath string, backend varlinkservice.Backend) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return err
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	server := varlink.NewServer()
	server.Handler = varlinkservice.Handler{
		Backend: backend,
	}
	go func() {
		if err := server.Serve(l); err != nil {
			fmt.Fprintf(os.Stderr, "failed to serve varlink server: %v\n", err)
		}
	}()
	return nil
}
