package z2m

// IsStateTopic exposes isStateTopic, whose false cases the public API can't
// observe: a misrouted topic is stored under an id no device resolves.
var IsStateTopic = isStateTopic

// HandleRetained processes a message as the broker's retained copy of topic,
// as sent on every (re)subscribe.
func (c *Client) HandleRetained(topic string, payload []byte) {
	c.handle(topic, payload, true)
}
