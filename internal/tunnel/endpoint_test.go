package tunnel

import (
	"testing"
)

func TestNewEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
	}{
		{
			name:     "host only",
			input:    "localhost",
			wantHost: "localhost",
			wantPort: 0,
		},
		{
			name:     "host with port",
			input:    "localhost:8080",
			wantHost: "localhost",
			wantPort: 8080,
		},
		{
			name:     "IP address with port",
			input:    "192.168.1.1:6443",
			wantHost: "192.168.1.1",
			wantPort: 6443,
		},
		{
			name:     "domain with port",
			input:    "example.com:443",
			wantHost: "example.com",
			wantPort: 443,
		},
		{
			name:     "empty string",
			input:    "",
			wantHost: "",
			wantPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := NewEndpoint(tt.input)

			if ep.Host != tt.wantHost {
				t.Errorf("NewEndpoint(%q).Host = %q, want %q", tt.input, ep.Host, tt.wantHost)
			}

			if ep.Port != tt.wantPort {
				t.Errorf("NewEndpoint(%q).Port = %d, want %d", tt.input, ep.Port, tt.wantPort)
			}
		})
	}
}

func TestNewEndpointWithPort(t *testing.T) {
	ep := NewEndpointWithPort("myhost", 9000)

	if ep.Host != "myhost" {
		t.Errorf("Host = %q, want %q", ep.Host, "myhost")
	}

	if ep.Port != 9000 {
		t.Errorf("Port = %d, want %d", ep.Port, 9000)
	}
}

func TestEndpointString(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{"localhost", "localhost", 8080, "localhost:8080"},
		{"IP address", "10.0.0.1", 6443, "10.0.0.1:6443"},
		{"domain", "example.com", 443, "example.com:443"},
		{"zero port", "localhost", 0, "localhost:0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := NewEndpointWithPort(tt.host, tt.port)

			if ep.String() != tt.expected {
				t.Errorf("String() = %q, want %q", ep.String(), tt.expected)
			}
		})
	}
}

func TestEndpointAddress(t *testing.T) {
	ep := NewEndpointWithPort("localhost", 8080)

	// Address should be same as String for this implementation
	if ep.Address() != ep.String() {
		t.Errorf("Address() = %q, String() = %q, should be equal", ep.Address(), ep.String())
	}
}

func TestNewEndpointParseInvalidPort(t *testing.T) {
	// Invalid port should result in 0
	ep := NewEndpoint("localhost:notaport")

	if ep.Host != "localhost" {
		t.Errorf("Host = %q, want %q", ep.Host, "localhost")
	}

	if ep.Port != 0 {
		t.Errorf("Port with invalid string = %d, want 0", ep.Port)
	}
}

func TestNewEndpointMultipleColons(t *testing.T) {
	// Edge case: multiple colons (like IPv6 without brackets)
	ep := NewEndpoint("::1:8080")

	// First split gives us "" and "" and "1:8080" etc.
	// The behavior might vary - just verify it doesn't crash
	t.Logf("Multiple colons: Host=%q, Port=%d", ep.Host, ep.Port)
}
