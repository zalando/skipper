package swarm

import (
	"bytes"
	"encoding/gob"
	"maps"
)

type messageType int

const (
	sharedValue messageType = iota
	broadcast
)

type message struct {
	Type   messageType
	Source string
	Key    string
	Value  any
}

type outgoingMessage struct {
	message *message
	encoded []byte
}

type Message struct {
	Source string
	Value  any
}

type reqOutgoing struct {
	overhead int
	limit    int
	ret      chan [][]byte
}

// mlDelegate is a memberlist delegate
type mlDelegate struct {
	meta     []byte
	outgoing chan<- reqOutgoing
	incoming chan<- []byte
}

type sharedValues map[string]map[string]any

type valueReq struct {
	key string
	ret chan map[string]any
}

// NodeMeta implements a memberlist delegate
func (d *mlDelegate) NodeMeta(limit int) []byte {
	if len(d.meta) > limit {
		// TODO: would nil better here?
		// documentation is unclear
		return d.meta[:limit]
	}

	return d.meta
}

// NotifyMsg implements a memberlist delegate
func (d *mlDelegate) NotifyMsg(m []byte) {
	d.incoming <- m
}

// GetBroadcasts implements a memberlist delegate
// TODO: verify over TCP-only
func (d *mlDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	req := reqOutgoing{
		overhead: overhead,
		limit:    limit,
		ret:      make(chan [][]byte),
	}
	d.outgoing <- req
	return <-req.ret
}

// LocalState implements a memberlist delegate
func (d *mlDelegate) LocalState(bool) []byte { return nil }

// MergeRemoteState implements a memberlist delegate
func (d *mlDelegate) MergeRemoteState(buf []byte, join bool) {}

// the top level map is used internally, we can use it as mutable
// the leaf maps are shared, we need to clone those
func (sv sharedValues) set(source, key string, value any) {
	prev := sv[key]
	sv[key] = make(map[string]any)
	maps.Copy(sv[key], prev)

	sv[key][source] = value
}

func encodeMessage(m *message) ([]byte, error) {
	// we're not saving the encoder together with the connections, because
	// even if the reflection info would be cached, it's very fragile and
	// complicated. These messages should be small, it should be OK to pay
	// this cost.

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(m)
	return buf.Bytes(), err
}

func decodeMessage(b []byte) (*message, error) {
	var m message
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	err := dec.Decode(&m)
	return &m, err
}
