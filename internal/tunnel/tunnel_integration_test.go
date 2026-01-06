//go:build integration

package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestIntegration_TunnelWithEphemeralSSH tests tunnel functionality with an ephemeral SSH server.
// This test requires TEST_INTEGRATION=1 environment variable.
func TestIntegration_TunnelWithEphemeralSSH(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test (set TEST_INTEGRATION=1 to run)")
	}

	// Start an ephemeral echo server as the "remote" endpoint
	echoServer, err := startEchoServer()
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer echoServer.Close()

	t.Logf("Echo server listening on %s", echoServer.Addr().String())

	// Start a mock SSH server that forwards to the echo server
	sshServer, err := startMockSSHServer(echoServer.Addr().String())
	if err != nil {
		t.Fatalf("Failed to start mock SSH server: %v", err)
	}
	defer sshServer.Close()

	t.Logf("Mock SSH server listening on %s", sshServer.Addr().String())

	// Create SSH tunnel through the mock server
	tunnel := &SSHTunnel{
		Local:  &Endpoint{Host: "localhost", Port: 0}, // Ephemeral port
		Server: parseEndpoint(sshServer.Addr().String()),
		Remote: parseEndpoint(echoServer.Addr().String()),
		Config: &ssh.ClientConfig{
			User:            "testuser",
			Auth:            []ssh.AuthMethod{ssh.Password("testpass")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		},
		SshConnectionPoolSize:         2,
		SshConnectionMaxConcurrentUse: 5,
		SshWarmupConnectionCount:      1,
		Ready:                         make(chan struct{}),
	}

	// Start tunnel in background
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tunnel.Start()
	}()

	// Close tunnel when context times out
	go func() {
		<-ctx.Done()
		tunnel.Close()
	}()

	// Wait for tunnel to be ready
	select {
	case <-tunnel.Ready:
		t.Log("Tunnel is ready")
	case err := <-errCh:
		t.Fatalf("Tunnel failed to start: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for tunnel to become ready")
	}

	// Test sending data through the tunnel
	localAddr := fmt.Sprintf("localhost:%d", tunnel.GetActualLocalPort())
	t.Logf("Tunnel listening on %s", localAddr)

	// Connect through the tunnel
	conn, err := net.DialTimeout("tcp", localAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect through tunnel: %v", err)
	}
	defer conn.Close()

	// Send test data
	testData := "Hello through tunnel!"
	_, err = conn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Failed to write through tunnel: %v", err)
	}

	// Read echoed response
	buf := make([]byte, len(testData))
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("Failed to read through tunnel: %v", err)
	}

	if string(buf) != testData {
		t.Errorf("Expected %q, got %q", testData, string(buf))
	}

	t.Log("Data successfully tunneled and echoed back")
}

// TestIntegration_TunnelConcurrentConnections tests multiple concurrent connections through tunnel.
func TestIntegration_TunnelConcurrentConnections(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "1" {
		t.Skip("Skipping integration test (set TEST_INTEGRATION=1 to run)")
	}

	echoServer, err := startEchoServer()
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer echoServer.Close()

	sshServer, err := startMockSSHServer(echoServer.Addr().String())
	if err != nil {
		t.Fatalf("Failed to start mock SSH server: %v", err)
	}
	defer sshServer.Close()

	tunnel := &SSHTunnel{
		Local:  &Endpoint{Host: "localhost", Port: 0},
		Server: parseEndpoint(sshServer.Addr().String()),
		Remote: parseEndpoint(echoServer.Addr().String()),
		Config: &ssh.ClientConfig{
			User:            "testuser",
			Auth:            []ssh.AuthMethod{ssh.Password("testpass")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		},
		SshConnectionPoolSize:         5,
		SshConnectionMaxConcurrentUse: 10,
		SshWarmupConnectionCount:      2,
		Ready:                         make(chan struct{}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	go func() {
		_ = tunnel.Start()
	}()

	// Close tunnel when context times out
	go func() {
		<-ctx.Done()
		tunnel.Close()
	}()

	<-tunnel.Ready

	// Run concurrent connections
	const numConnections = 20
	var wg sync.WaitGroup
	errors := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp",
				fmt.Sprintf("localhost:%d", tunnel.GetActualLocalPort()),
				5*time.Second)
			if err != nil {
				errors <- fmt.Errorf("connection %d: dial failed: %w", id, err)
				return
			}
			defer conn.Close()

			testData := fmt.Sprintf("message-%d", id)
			if _, err := conn.Write([]byte(testData)); err != nil {
				errors <- fmt.Errorf("connection %d: write failed: %w", id, err)
				return
			}

			buf := make([]byte, len(testData))
			if _, err := io.ReadFull(conn, buf); err != nil {
				errors <- fmt.Errorf("connection %d: read failed: %w", id, err)
				return
			}

			if string(buf) != testData {
				errors <- fmt.Errorf("connection %d: expected %q, got %q", id, testData, string(buf))
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		t.Error(err)
		errCount++
	}

	if errCount > 0 {
		t.Errorf("%d/%d connections failed", errCount, numConnections)
	} else {
		t.Logf("All %d concurrent connections succeeded", numConnections)
	}
}

// Helper functions

func parseEndpoint(addr string) *Endpoint {
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return &Endpoint{Host: host, Port: port}
}

func startEchoServer() (net.Listener, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	return listener, nil
}

// mockSSHServer is a minimal SSH server for testing.
type mockSSHServer struct {
	listener   net.Listener
	forwardTo  string
	privateKey ssh.Signer
}

func startMockSSHServer(forwardTo string) (*mockSSHServer, error) {
	// Generate a test key (in real tests, use a pre-generated key)
	privateKey, err := generateTestKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate test key: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	server := &mockSSHServer{
		listener:   listener,
		forwardTo:  forwardTo,
		privateKey: privateKey,
	}

	go server.serve()

	return server, nil
}

func (s *mockSSHServer) serve() {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "testuser" && string(pass) == "testpass" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}
	config.AddHostKey(s.privateKey)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConnection(conn, config)
	}
}

func (s *mockSSHServer) handleConnection(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()

	// Handle global requests (like keepalives)
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() == "direct-tcpip" {
			go s.handleDirectTCPIP(newChannel)
		} else {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
		}
	}
}

func (s *mockSSHServer) handleDirectTCPIP(newChannel ssh.NewChannel) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close()

	go ssh.DiscardRequests(requests)

	// Connect to the target
	target, err := net.Dial("tcp", s.forwardTo)
	if err != nil {
		return
	}
	defer target.Close()

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(target, channel)
	}()

	go func() {
		defer wg.Done()
		io.Copy(channel, target)
	}()

	wg.Wait()
}

func (s *mockSSHServer) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *mockSSHServer) Close() error {
	return s.listener.Close()
}

func generateTestKey() (ssh.Signer, error) {
	// Use a hardcoded test key for reproducibility
	// In production tests, generate or load from a file
	testKey := `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBHK2VfZPBOQNbBa5fKGZLnT8TpYXYZhPIVFmvvMhPQhQAAAJBl8CtlZfAr
ZQAAAAtzc2gtZWQyNTUxOQAAACBHK2VfZPBOQNbBa5fKGZLnT8TpYXYZhPIVFmvvMhPQhQ
AAAEBLS5/KFLYd2T7l8kXOkPMXHNqxzxmqVk+sP/xqVCxxREcrZV9k8E5A1sFrl8oZkudP
xOlhdhmE8hUWa+8yE9CFAAAADnRlc3RAZXhhbXBsZS5jb20BAgMEBQY=
-----END OPENSSH PRIVATE KEY-----`

	signer, err := ssh.ParsePrivateKey([]byte(testKey))
	if err != nil {
		return nil, err
	}
	return signer, nil
}
