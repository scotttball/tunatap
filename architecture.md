# Tunatap Architecture

## Overview

Tunatap is a CLI tool that creates SSH tunnels through OCI Bastion services to access private resources like OKE clusters. It manages bastion sessions, SSH connections, and port forwarding with connection pooling for efficiency.

### Zero-Touch Architecture

Tunatap supports a "zero-touch" mode that requires no configuration file:

1. **Dynamic Discovery**: Searches all compartments across all subscribed regions to find clusters by name
2. **Ephemeral SSH Keys**: Generates ED25519 key pairs in memory for each session (no static key files)
3. **Intelligent Caching**: Caches discovery results in `~/.tunatap/cache.json` for fast subsequent connections
4. **Backwards Compatible**: Falls back to config file if present, discovery only runs if cluster not found in config

## High-Level Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              User Machine                                    │
│                                                                              │
│  ┌──────────┐    ┌─────────────────────────────────────────────────────┐    │
│  │  kubectl │───▶│ localhost:6443                                       │    │
│  └──────────┘    │     ▼                                                │    │
│                  │ ┌─────────────┐                                      │    │
│                  │ │ TCP Listener │                                      │    │
│                  │ └──────┬──────┘                                      │    │
│                  │        ▼                                              │    │
│                  │ ┌─────────────────┐    ┌─────────────────────────┐   │    │
│                  │ │ Connection Pool │───▶│ SSH Connections (1..N)  │   │    │
│                  │ └─────────────────┘    └───────────┬─────────────┘   │    │
│                  │                                     │                 │    │
│                  └─────────────────────────────────────┼─────────────────┘    │
│                                                        │                      │
└────────────────────────────────────────────────────────┼──────────────────────┘
                                                         │ SSH Tunnel
                                                         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              OCI Bastion                                     │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │ session-xxx.bastion.<region>.oci.oraclecloud.com:22                │     │
│  └────────────────────────────────────────────────────────────────────┘     │
│                                    │                                         │
└────────────────────────────────────┼─────────────────────────────────────────┘
                                     │ Port Forward
                                     ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Private Subnet                                     │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │ OKE Cluster API Server (10.x.x.x:6443)                             │     │
│  └────────────────────────────────────────────────────────────────────┘     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Project Structure

```
tunatap/
├── main.go                      # Entry point, initializes logging and config directory
├── go.mod                       # Module definition and dependencies (Go 1.21+)
├── Makefile                     # Build, test, lint, and release commands
├── .golangci.yml                # Linter configuration
├── .goreleaser.yml              # Release automation
│
├── cmd/                         # CLI commands (Cobra)
│   ├── root.go                  # Root command, global flags, logging setup
│   ├── connect.go               # Connect command - main tunnel functionality
│   ├── exec.go                  # Exec command - run commands with tunnel+kubeconfig
│   ├── cache.go                 # Cache management (show, clear)
│   ├── setup.go                 # Setup wizard and config subcommands
│   ├── list.go                  # List clusters and bastions
│   ├── doctor.go                # Diagnostic checks
│   ├── catalog.go               # Cluster catalog management
│   ├── audit.go                 # Configuration auditing
│   └── version.go               # Version information
│
├── internal/                    # Private implementation packages (16 packages)
│   ├── config/                  # Configuration management
│   │   ├── config.go            # Type definitions (Config, Cluster, Endpoint)
│   │   ├── reader.go            # YAML read/write operations
│   │   └── globals.go           # Remote config loading from OCI Object Storage
│   │
│   ├── discovery/               # Zero-touch cluster discovery (NEW)
│   │   ├── discovery.go         # Main discovery orchestration
│   │   ├── cache.go             # Cache manager (~/.tunatap/cache.json)
│   │   └── compartment.go       # Recursive compartment traversal
│   │
│   ├── sshkeys/                 # Ephemeral SSH key generation (NEW)
│   │   └── ephemeral.go         # ED25519 key pair generation in memory
│   │
│   ├── tunnel/                  # SSH tunnel implementation
│   │   ├── endpoint.go          # Network endpoint abstraction
│   │   ├── tunnel.go            # SSHTunnel - listener and forwarder
│   │   └── ssh.go               # SSH client configuration helpers
│   │
│   ├── pool/                    # Connection pooling
│   │   ├── pool.go              # ConnectionPool - manages multiple connections
│   │   └── connection.go        # TrackedSSHConnection - usage tracking
│   │
│   ├── bastion/                 # OCI Bastion integration
│   │   ├── bastion.go           # Main tunnel orchestration
│   │   ├── session.go           # Bastion session lifecycle (SessionManager)
│   │   └── commands.go          # SSH command string generation
│   │
│   ├── cluster/                 # Cluster operations
│   │   └── cluster.go           # Validation, OCID lookup, port allocation
│   │
│   ├── client/                  # OCI SDK wrapper
│   │   ├── oci.go               # OCIClient - unified OCI API access
│   │   ├── interface.go         # OCIClientInterface for testing
│   │   └── mock_client.go       # Mock implementation for tests
│   │
│   ├── kubeconfig/              # Kubernetes configuration
│   │   └── kubeconfig.go        # Generate/inject kubeconfig for clusters
│   │
│   ├── catalog/                 # Cluster catalog management
│   │   └── catalog.go           # Remote catalog sources and syncing
│   │
│   ├── audit/                   # Configuration auditing
│   │   └── audit.go             # Audit checks and reporting
│   │
│   ├── autofix/                 # Automatic problem resolution
│   │   └── autofix.go           # Fix common configuration issues
│   │
│   ├── preflight/               # Pre-connection diagnostics
│   │   └── preflight.go         # Connectivity and config checks
│   │
│   └── state/                   # Application state
│       └── state.go             # Global state singleton
│
├── pkg/                         # Public/reusable packages
│   └── utils/
│       ├── utils.go             # Pointer helper functions (StringPtr, BoolPtr, IntPtr)
│       └── paths.go             # Cross-platform path utilities
│
└── .github/workflows/           # CI/CD configuration
    ├── ci.yml                   # Build, test, lint on PR/push
    └── release.yml              # Automated releases on tag
```

## Package Details

### cmd/

Command-line interface using Cobra framework.

**root.go**: Defines the root command with persistent flags (`--config`, `--debug`, `--raw`). Sets up zerolog logging and initializes global state.

**connect.go**: Main functionality. Loads config, resolves cluster, creates OCI client, validates cluster configuration, and starts the bastion tunnel. Uses go-fzf for interactive cluster selection.

**setup.go**: Interactive configuration wizard. Prompts for SSH key, SOCKS proxy, and cluster details. Subcommands: `init`, `show`, `add-cluster`, `add-tenancy`.

### internal/discovery/

Zero-touch cluster discovery system.

**Discoverer**: Main discovery orchestration
- `DiscoverCluster(ctx, name)`: Searches all compartments across all regions
- `DiscoverBastion(ctx, cluster)`: Finds bastion that can reach cluster
- `ResolveToConfig(discovered, bastion)`: Converts to `config.Cluster`

**Cache**: TTL-based caching at `~/.tunatap/cache.json`
- `GetCluster(name)` / `SetCluster(name, entry)`: Cluster caching
- `GetBastion(name)` / `SetBastion(name, entry)`: Bastion caching
- `Invalidate(name)` / `InvalidateAll()`: Cache clearing
- Default TTL: 24 hours

**CompartmentTree**: Recursive compartment traversal
- `BuildCompartmentTree(ctx, ociClient, tenancyID)`: Build full hierarchy
- `ForEachParallel(ctx, workers, fn)`: Parallel traversal with rate limiting

**Discovery Algorithm**:
```
1. Check cache → return if valid
2. Get tenancy from OCI config provider
3. Get subscribed regions (home region searched first)
4. For each region:
   a. Build compartment tree
   b. Search each compartment in parallel
   c. Match cluster by name (case-insensitive)
5. Get full cluster details (VCN, subnet, endpoint)
6. Find bastion in cluster's compartment
7. Cache results and return
```

### internal/sshkeys/

Ephemeral SSH key generation for bastion sessions.

**EphemeralKeyPair**: In-memory ED25519 keys
- `GenerateEphemeralKeyPair()`: Creates new key pair
- `Signer()`: Returns `ssh.Signer` for tunnel auth
- `PublicKeyString()`: Returns OpenSSH format for OCI session
- `AuthMethod()`: Returns `ssh.AuthMethod` for client config

Keys are never written to disk - generated fresh for each session.

### internal/config/

Configuration types and persistence.

**Config struct hierarchy**:
```
Config
├── Tenancies: map[string]*string     # name -> OCID mapping
├── Clusters: []*Cluster
│   ├── ClusterName, Region
│   ├── Ocid (or Tenant + Compartment for lookup)
│   ├── BastionId, BastionType, Bastion
│   ├── LocalPort
│   └── Endpoints: []*ClusterEndpoint
│       ├── Name ("private", "public")
│       ├── Ip
│       └── Port
├── RemoteConfig                      # Optional OCI Object Storage config
└── SSH settings (pool size, warmup, max concurrent)
```

### internal/tunnel/

Core SSH tunneling implementation.

**SSHTunnel**: Listens on a local port and forwards connections through an SSH tunnel.

```
SSHTunnel
├── Local     *Endpoint    # localhost:6443
├── Server    *Endpoint    # bastion host
├── Remote    *Endpoint    # cluster API server
├── Config    *ssh.ClientConfig
└── Pool settings
```

**Start()** flow:
1. Create TCP listener on local endpoint
2. Initialize connection pool with SSH factory
3. Start health check goroutine
4. Accept connections and forward through pool

**forward()** flow:
1. Get tracked connection from pool
2. Dial remote endpoint through SSH
3. Bidirectional pipe between local and remote

### internal/pool/

SSH connection pooling for efficiency.

**ConnectionPool**: Manages multiple SSH connections with limits.
- `maxSize`: Maximum connections in pool
- `maxConcurrent`: Max simultaneous uses per connection
- Warmup: Pre-establishes connections on init

**TrackedSSHConnection**: Wraps ssh.Client with usage tracking.
- `useCount`: Current active uses
- `invalid`: Marked when connection fails
- Thread-safe increment/decrement

**Get()** algorithm:
1. Find existing connection with capacity → reuse
2. Remove invalid connections
3. If pool not full → create new connection
4. Otherwise → error (pool exhausted)

### internal/bastion/

OCI Bastion service integration.

**Two bastion types**:

1. **STANDARD**: Uses OCI Bastion service
   - Creates/retrieves bastion session via OCI API
   - Session provides SSH endpoint at `session-xxx.bastion.<region>.oci.oraclecloud.com`
   - Sessions expire and need refresh (30-second ticker)

2. **INTERNAL**: Uses internal bastion with jump box
   - Connects to `ztb-internal.bastion.<region>.oci.oracleiaas.com`
   - Requires `jumpbox_ip` configuration
   - Uses ProxyJump through the jump box

**TunnelThroughBastion()**: Retry loop (30 attempts with exponential backoff) that:
1. Gets/refreshes bastion session
2. Establishes SSH tunnel
3. On failure: increment backoff and retry

### internal/client/

OCI SDK wrapper providing unified API access.

**OCIClient** wraps:
- `identityClient`: Tenancy, compartment lookups
- `objectStorageClient`: Remote config fetching
- `containerEngineClient`: OKE cluster operations
- `bastionClient`: Bastion session management

**Authentication Methods** (5 supported):
1. `config` - Standard OCI config file (`~/.oci/config`)
2. `instance_principal` - For OCI compute instances
3. `resource_principal` - For OCI Functions
4. `security_token` - SSO/SAML token-based auth
5. `auto` - Auto-detection in priority order

Key methods:
- `NewOCIClientWithAuthType()`: Factory with auth type dispatch
- `NewOCIClientAuto()`: Auto-detection with fallback chain
- `GetCompartmentIdByPath()`: Resolves "parent/child" path to OCID
- `FetchClusterId()`: Finds cluster OCID by name
- `CreateSession()`, `GetSession()`, `WaitForSessionActive()`: Session lifecycle
- `ListBastions()`: Lists bastions in compartment
- `GetObject()`: Fetch remote config from Object Storage

### internal/state/

Global application state singleton.

Stores:
- Home path (`~/.tunatap`)
- Tenancy name-to-OCID mappings

### internal/kubeconfig/

Kubernetes configuration management.

- Generates kubeconfig files for connected clusters
- Injects tunnel endpoints into existing kubeconfig
- Supports multiple cluster contexts

### internal/catalog/

Cluster catalog management for team sharing.

- Syncs cluster definitions from remote sources
- Supports OCI Object Storage as catalog backend
- Merges remote clusters with local configuration

### internal/audit/

Configuration auditing and validation.

- Checks for configuration inconsistencies
- Validates cluster and bastion settings
- Reports potential issues

### internal/autofix/

Automatic problem resolution.

- Fixes common configuration issues
- Repairs broken cluster entries
- Updates deprecated settings

### internal/preflight/

Pre-connection diagnostic checks.

- Validates OCI credentials
- Tests SSH key accessibility
- Checks network connectivity
- Verifies bastion accessibility

### pkg/utils/

Cross-platform utilities.

**paths.go**: Platform-agnostic path helpers
- `HomeDir()`: Returns user home directory
- `ExpandPath()`: Expands `~` to home directory
- `DefaultSSHDir()`, `DefaultOCIConfigPath()`, etc.
- `IsWindows()`, `IsMac()`, `IsLinux()`: Platform detection

**utils.go**: Pointer helper functions
- `StringPtr()`, `BoolPtr()`, `IntPtr()`: Create pointers from values

## Connection Lifecycle

### Zero-Touch Mode

```
1. Discovery
   ├── Check cache for cluster → return if valid
   ├── Get tenancy OCID from OCI config provider
   ├── Get subscribed regions
   ├── For each region (home first):
   │   ├── Build compartment tree
   │   ├── Search compartments in parallel
   │   └── Match cluster by name
   ├── Get cluster details (VCN, subnet, endpoint)
   ├── Find bastion in cluster's compartment
   └── Cache results

2. Key Generation
   └── Generate ephemeral ED25519 key pair (in memory)

3. Session Setup
   ├── Create bastion session with ephemeral public key
   ├── Wait for session to become ACTIVE
   └── Get session SSH endpoint

4. Tunnel Establishment
   ├── Create connection pool with ephemeral signer
   ├── Start local TCP listener
   └── Start health check goroutine

[Continue with steps 4-6 below...]
```

### Traditional Mode (with config file)

```
1. Startup
   ├── Load ~/.tunatap/config.yaml
   ├── Initialize OCI client with ~/.oci/config
   └── Validate cluster configuration (resolve OCIDs if needed)

2. Session Setup
   ├── Find or create bastion session via OCI API
   ├── Wait for session to become ACTIVE
   └── Get session SSH endpoint

3. Tunnel Establishment
   ├── Create connection pool (warmup N connections)
   ├── Start local TCP listener
   └── Start health check goroutine
```

### Common Steps (both modes)

```
4. Request Forwarding (per connection)
   ├── Accept local connection
   ├── Get SSH connection from pool
   ├── Dial remote through SSH
   └── Bidirectional pipe until close

5. Maintenance
   ├── Health check (10s interval): keepalive probe
   ├── Session refresh (30s interval): extend/recreate session
   └── Connection cleanup: remove failed connections

6. Shutdown
   ├── Cancel context
   ├── Close all pool connections
   └── Close listener
```

## Error Handling

- **Retry with backoff**: Bastion tunnel failures retry up to 30 times with increasing delays
- **Connection invalidation**: Failed connections are marked invalid and removed on next pool operation
- **Health checks**: Periodic keepalive probes detect dead connections
- **Graceful shutdown**: SIGINT/SIGTERM handling closes tunnels cleanly

## Concurrency Patterns

### Mutex Usage
- `sync.Mutex` protects: ConnectionPool, TrackedSSHConnection, State, SessionManager
- `sync.RWMutex` for read-heavy state access

### Goroutines
- Connection listener goroutine per tunnel
- Health check goroutine (10s interval)
- Bidirectional pipe goroutines for forwarding
- Session refresh goroutine (30s interval)

### Channels
- `Ready` channel signals tunnel readiness
- Error channels for async operation results
- Context cancellation for graceful shutdown

### Atomic Operations
- `atomic.AddInt32` for concurrent call counting in tests
- Thread-safe connection use tracking

## Testing

### Test Coverage
- 19 test files across all major packages
- ~2,500 lines of test code
- Unit tests with mock factories (no external mocking libraries)

### Testing Patterns
```go
// Mock factory pattern (pool_test.go)
func mockFactory(shouldFail bool) ConnectionFactory {
    return func() (*ssh.Client, error) {
        if shouldFail {
            return nil, errors.New("mock failure")
        }
        return nil, nil  // nil client works for most tests
    }
}
```

### Running Tests
```bash
go test ./...                    # All tests
go test ./... -race              # With race detection
go test ./internal/pool -v       # Specific package, verbose
go test ./... -cover             # With coverage
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/oracle/oci-go-sdk/v65` | OCI API access |
| `golang.org/x/crypto/ssh` | SSH client implementation |
| `golang.org/x/net/proxy` | SOCKS proxy support |
| `github.com/rs/zerolog` | Structured logging |
| `gopkg.in/yaml.v3` | Configuration parsing |
| `github.com/koki-develop/go-fzf` | Interactive selection |
| `charmbracelet/bubbletea` | Terminal UI components |
| `charmbracelet/lipgloss` | Terminal styling |
