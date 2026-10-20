package z2m

// HandleRetained processes a message as the broker's retained copy of topic,
// as sent on every (re)subscribe.
func (c *Client) HandleRetained(topic string, payload []byte) {
	c.handle(topic, payload, true)
}
