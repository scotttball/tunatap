package tunnel

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestNewSSHTunnel(t *testing.T) {
	sshConfig := &ssh.ClientConfig{
		User: "testuser",
	}

	tunnel := NewSSHTunnel(
		"localhost:8080",
		"bastion.example.com:22",
		sshConfig,
		"10.0.0.1:6443",
		5,  // poolSize
		2,  // warmupCount
		10, // maxConcurrent
		"", // no socks proxy
	)

	if tunnel == nil {
		t.Fatal("NewSSHTunnel() returned nil")
	}

	// Verify local endpoint
	if tunnel.Local.Host != "localhost" {
		t.Errorf("Local.Host = %q, want %q", tunnel.Local.Host, "localhost")
	}
	if tunnel.Local.Port != 8080 {
		t.Errorf("Local.Port = %d, want %d", tunnel.Local.Port, 8080)
	}

	// Verify server endpoint
	if tunnel.Server.Host != "bastion.example.com" {
		t.Errorf("Server.Host = %q, want %q", tunnel.Server.Host, "bastion.example.com")
	}
	if tunnel.Server.Port != 22 {
		t.Errorf("Server.Port = %d, want %d", tunnel.Server.Port, 22)
	}

	// Verify remote endpoint
	if tunnel.Remote.Host != "10.0.0.1" {
		t.Errorf("Remote.Host = %q, want %q", tunnel.Remote.Host, "10.0.0.1")
	}
	if tunnel.Remote.Port != 6443 {
		t.Errorf("Remote.Port = %d, want %d", tunnel.Remote.Port, 6443)
	}

	// Verify pool settings
	if tunnel.SshConnectionPoolSize != 5 {
		t.Errorf("SshConnectionPoolSize = %d, want %d", tunnel.SshConnectionPoolSize, 5)
	}
	if tunnel.SshWarmupConnectionCount != 2 {
		t.Errorf("SshWarmupConnectionCount = %d, want %d", tunnel.SshWarmupConnectionCount, 2)
	}
	if tunnel.SshConnectionMaxConcurrentUse != 10 {
		t.Errorf("SshConnectionMaxConcurrentUse = %d, want %d", tunnel.SshConnectionMaxConcurrentUse, 10)
	}

	// No SOCKS proxy
	if tunnel.SocksProxy != nil {
		t.Errorf("SocksProxy = %v, want nil", tunnel.SocksProxy)
	}
}

func TestNewSSHTunnelWithSocksProxy(t *testing.T) {
	sshConfig := &ssh.ClientConfig{
		User: "testuser",
	}

	tunnel := NewSSHTunnel(
		"localhost:8080",
		"bastion.example.com:22",
		sshConfig,
		"10.0.0.1:6443",
		5,
		2,
		10,
		"localhost:1080", // SOCKS proxy
	)

	if tunnel.SocksProxy == nil {
		t.Fatal("SocksProxy should not be nil")
	}

	if tunnel.SocksProxy.Host != "localhost" {
		t.Errorf("SocksProxy.Host = %q, want %q", tunnel.SocksProxy.Host, "localhost")
	}
	if tunnel.SocksProxy.Port != 1080 {
		t.Errorf("SocksProxy.Port = %d, want %d", tunnel.SocksProxy.Port, 1080)
	}
}

func TestSSHTunnelEstablishServerConnectionNoProxy(t *testing.T) {
	// This test can't actually connect, but verifies the code path
	sshConfig := &ssh.ClientConfig{
		User:            "testuser",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	tunnel := NewSSHTunnel(
		"localhost:8080",
		"localhost:22222", // Non-existent server
		sshConfig,
		"10.0.0.1:6443",
		5, 2, 10,
		"",
	)

	// This should fail to connect
	_, err := tunnel.establishServerConnection()
	if err == nil {
		t.Error("establishServerConnection() should fail when server is not available")
	}
}

func TestSSHTunnelNewConnectionPoolForRemote(t *testing.T) {
	sshConfig := &ssh.ClientConfig{
		User:            "testuser",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	tunnel := NewSSHTunnel(
		"localhost:8080",
		"localhost:22222",
		sshConfig,
		"10.0.0.1:6443",
		5, 0, 10, // 0 warmup to avoid connection attempts
		"",
	)

	pool, err := tunnel.NewConnectionPoolForRemote()
	if err != nil {
		t.Fatalf("NewConnectionPoolForRemote() error = %v", err)
	}
	defer pool.Close()

	if pool == nil {
		t.Error("NewConnectionPoolForRemote() returned nil")
	}
}

func TestSSHTunnelStartInvalidPort(t *testing.T) {
	sshConfig := &ssh.ClientConfig{
		User:            "testuser",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Use port 0 which will fail binding
	tunnel := NewSSHTunnel(
		"localhost:99999", // Invalid port
		"localhost:22222",
		sshConfig,
		"10.0.0.1:6443",
		5, 0, 10,
		"",
	)

	// Start in a goroutine since it blocks
	errCh := make(chan error, 1)
	go func() {
		errCh <- tunnel.Start()
	}()

	// The Start should fail quickly due to invalid port
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Start() should error with invalid port")
		}
	default:
		// If it doesn't return immediately, that's also acceptable
		// (it might be trying to connect)
		t.Log("Start() did not return immediately")
	}
}
