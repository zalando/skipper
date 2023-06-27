package kubernetes

import "time"

func (c *Client) SetLoggingInterval(d time.Duration) {
	c.loggingInterval = d
}
