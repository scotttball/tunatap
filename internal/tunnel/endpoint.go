package tunnel

import (
	"fmt"
	"strconv"
	"strings"
)

// Endpoint represents a network endpoint (host:port).
type Endpoint struct {
	Host string
	Port int
}

// NewEndpoint parses an endpoint string (host:port or just host).
func NewEndpoint(s string) *Endpoint {
	endpoint := &Endpoint{
		Host: s,
	}

	if parts := strings.Split(endpoint.Host, ":"); len(parts) > 1 {
		endpoint.Host = parts[0]
		endpoint.Port, _ = strconv.Atoi(parts[1])
	}

	return endpoint
}

// NewEndpointWithPort creates an endpoint with explicit host and port.
func NewEndpointWithPort(host string, port int) *Endpoint {
	return &Endpoint{
		Host: host,
		Port: port,
	}
}

// String returns the endpoint as host:port.
func (endpoint *Endpoint) String() string {
	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}

// Address returns just the host:port suitable for net.Dial.
func (endpoint *Endpoint) Address() string {
	return endpoint.String()
}
