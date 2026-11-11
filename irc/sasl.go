package irc

import (
	"bytes"
	"encoding/base64"
	"errors"
)

type SASLClient interface {
	Early() bool
	Handshake() (mech string)
	Respond(challenge string) (res string, err error)
}

type SASLPlain struct {
	Username string
	Password string
}

func (auth *SASLPlain) Early() bool {
	return true
}

func (auth *SASLPlain) Handshake() (mech string) {
	mech = "PLAIN"
	return
}

func (auth *SASLPlain) Respond(challenge string) (res string, err error) {
	if challenge != "+" {
		err = errors.New("unexpected challenge")
		return
	}

	user := []byte(auth.Username)
	pass := []byte(auth.Password)
	payload := bytes.Join([][]byte{user, user, pass}, []byte{0})
	res = base64.StdEncoding.EncodeToString(payload)

	return
}
