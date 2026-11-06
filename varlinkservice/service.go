// Package varlinkservice exposes the local OpenLink interface.
package varlinkservice

import (
	_ "embed"
	"encoding/json"

	govarlink "github.com/emersion/go-varlink"
)

//go:embed fr.delthas.senpai.varlink
var definition string

type OpenLinkIn struct {
	Link string `json:"link"`
}
type OpenLinkOut struct{}

type Client struct {
	*govarlink.Client
}

func (c Client) OpenLink(in *OpenLinkIn) (*OpenLinkOut, error) {
	if in == nil {
		in = new(OpenLinkIn)
	}
	out := new(OpenLinkOut)
	err := c.Client.Do("fr.delthas.senpai.OpenLink", in, out)
	return out, err
}

type Backend interface {
	OpenLink(*OpenLinkIn) (*OpenLinkOut, error)
}

type Handler struct {
	Backend Backend
}

func (h Handler) HandleVarlink(call *govarlink.ServerCall, req *govarlink.ServerRequest) error {
	var (
		out interface{}
		err error
	)
	switch req.Method {
	case "fr.delthas.senpai.OpenLink":
		in := new(OpenLinkIn)
		if err := json.Unmarshal(req.Parameters, in); err != nil {
			return err
		}
		out, err = h.Backend.OpenLink(in)
	default:
		err = &govarlink.ServerError{
			Name:       "org.varlink.service.MethodNotFound",
			Parameters: map[string]string{"method": req.Method},
		}
	}
	if err != nil {
		return err
	}
	return call.CloseWithReply(out)
}

func (h Handler) Register(reg *govarlink.Registry) {
	reg.Add(&govarlink.RegistryInterface{
		Definition: definition,
		Name:       "fr.delthas.senpai",
	}, h)
}
