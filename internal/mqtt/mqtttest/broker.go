package mqtttest

import (
	"net"
	"sync"
	"testing"

	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/stretchr/testify/require"
)

// Broker is just enough of an MQTT broker on loopback for a real paho client:
// it accepts every CONNECT, acknowledges SUBSCRIBE and PINGREQ, and records
// the topic of every PUBLISH. After each SUBACK it sends every message stored
// with Retain, flagged retained, and once a connection has subscribed it
// forwards that connection's own publishes back to it, not retained. Topic
// filters are not matched.
type Broker struct {
	ln net.Listener

	mu       sync.Mutex
	conns    []net.Conn
	topics   []string
	retained []message
}

// message is a stored or forwarded MQTT message.
type message struct {
	topic    string
	payload  string
	retained bool
}

// publishPacket builds a fresh QoS 0 PUBLISH for m; packets are not shared
// between connections, since writing one mutates it.
func publishPacket(m message) *packets.PublishPacket {
	p := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	p.TopicName = m.topic
	p.Payload = []byte(m.payload)
	p.Retain = m.retained
	return p
}

// StartBroker listens on addr ("127.0.0.1:0" for any free port) and stops
// when the test ends.
func StartBroker(t testing.TB, addr string) *Broker {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	b := &Broker{ln: ln}
	go b.accept()
	t.Cleanup(b.Stop)
	return b
}

// FreeAddr returns a loopback address nothing listens on, so connecting to it
// is refused until a Broker is started there.
func FreeAddr(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// Addr returns the address the broker listens on.
func (b *Broker) Addr() string { return b.ln.Addr().String() }

// URL returns the broker URL to connect to, "tcp://" + Addr.
func (b *Broker) URL() string { return "tcp://" + b.Addr() }

// Stop closes the listener and drops every connection, like a broker going
// down. It is safe to call more than once.
func (b *Broker) Stop() {
	_ = b.ln.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, conn := range b.conns {
		_ = conn.Close()
	}
}

// Retain stores a message under its full topic, to be sent after every later
// SUBACK.
func (b *Broker) Retain(topic, payload string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.retained = append(b.retained, message{topic: topic, payload: payload, retained: true})
}

// Published returns the topics of the PUBLISH packets received so far.
func (b *Broker) Published() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.topics...)
}

func (b *Broker) accept() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.conns = append(b.conns, conn)
		b.mu.Unlock()
		go b.serve(conn)
	}
}

// serve handles one client connection until it closes. It is the only writer
// to conn.
func (b *Broker) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	subscribed := false
	for {
		pkt, err := packets.ReadPacket(conn)
		if err != nil {
			return
		}
		var replies []packets.ControlPacket
		switch p := pkt.(type) {
		case *packets.ConnectPacket:
			replies = append(replies, packets.NewControlPacket(packets.Connack))
		case *packets.SubscribePacket:
			ack := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
			ack.MessageID = p.MessageID
			ack.ReturnCodes = make([]byte, len(p.Topics))
			replies = append(replies, ack)
			b.mu.Lock()
			for _, m := range b.retained {
				replies = append(replies, publishPacket(m))
			}
			b.mu.Unlock()
			subscribed = true
		case *packets.PingreqPacket:
			replies = append(replies, packets.NewControlPacket(packets.Pingresp))
		case *packets.PublishPacket:
			b.mu.Lock()
			b.topics = append(b.topics, p.TopicName)
			b.mu.Unlock()
			if subscribed {
				replies = append(replies, publishPacket(message{topic: p.TopicName, payload: string(p.Payload)}))
			}
		case *packets.DisconnectPacket:
			return
		}
		for _, reply := range replies {
			if err := reply.Write(conn); err != nil {
				return
			}
		}
	}
}
