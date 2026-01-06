# Tunatap

```
   ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
        \        \        \        \        \
         \        \        \        \        \
   ~~~~~~~\~~~~~~~~\~~~~~~~~\~~~~~~~~\~~~~~~~~\~~~
            \        \        \        \
             \        \        \        \
   <><        \        \        \        \       <><
   -----------========== TUNATAP ==========-----------
              secure tunnels beneath the surface
```

SSH tunnel manager for OCI Bastion services. Simplifies connecting to private OKE (Oracle Kubernetes Engine) clusters and other private resources through OCI Bastion hosts.

## Features

- **Zero-Touch Mode**: Connect to clusters by name without any configuration file
- **Dynamic Discovery**: Automatically discovers clusters across all compartments and regions
- **Ephemeral SSH Keys**: In-memory ED25519 key generation (no static key files needed)
- **Intelligent Caching**: Discovery results cached for fast subsequent connections
- **SSH Tunneling**: Establish secure tunnels through OCI Bastion services
- **Connection Pooling**: Efficient SSH connection reuse with configurable pool size
- **Bastion Types**: Support for both STANDARD (OCI Bastion service) and INTERNAL (jump box) modes
- **Interactive Selection**: Fuzzy finder (fzf) for cluster selection
- **SOCKS Proxy**: Route SSH connections through SOCKS proxies
- **Cross-Platform**: Full support for Linux, macOS, and Windows
- **Multiple Auth Methods**: OCI config file, instance principal, resource principal, security token, auto-detect
- **Kubeconfig Injection**: Automatic kubeconfig generation for connected clusters
- **Exec Pattern**: Run commands with tunnel and kubeconfig automatically configured
- **Remote Config**: Load shared cluster catalogs from OCI Object Storage
- **Health Monitoring**: Automatic connection health checks with keepalive probes
- **Health Endpoint**: Optional HTTP server for Kubernetes probes and Prometheus metrics
- **Session Management**: Automatic bastion session refresh before expiration
- **Preflight Checks**: Built-in diagnostics for troubleshooting connectivity
- **Status & Logs**: Real-time tunnel status and activity log streaming
- **Resilient Retries**: Exponential backoff with jitter for robust connections

## How It Works

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

## Installation

### Go Install (Recommended)

If you have Go 1.21+ installed:

```bash
go install github.com/scotttball/tunatap@latest
```

### Download Binary

Download pre-built binaries from the [Releases](https://github.com/scotttball/tunatap/releases) page.

**Linux/macOS:**
```bash
# Set version (check releases page for latest)
VERSION="2026.01.1"

# Download for your platform (linux_amd64, linux_arm64, darwin_amd64, darwin_arm64)
curl -LO "https://github.com/scotttball/tunatap/releases/download/${VERSION}/tunatap_${VERSION}_linux_amd64.tar.gz"
tar xzf tunatap_${VERSION}_linux_amd64.tar.gz
sudo mv tunatap /usr/local/bin/

# Verify installation
tunatap version
```

**Windows:**
1. Download the `.zip` file from Releases
2. Extract `tunatap.exe`
3. Add to your PATH or move to a directory in your PATH

### Build from Source

```bash
git clone https://github.com/scotttball/tunatap.git
cd tunatap
go build -o tunatap .

# Optional: Install to GOPATH/bin
go install .
```

## Prerequisites

- OCI CLI configured (`~/.oci/config`) - that's it for zero-touch mode!
- (Optional) SSH key pair for bastion authentication (ephemeral keys used by default)
- Access to OCI Bastion service in your tenancy

## Quick Start

### Zero-Touch Mode (Recommended)

No configuration file needed! Just ensure your OCI CLI is configured:

```bash
# Connect to any cluster by name - tunatap discovers it automatically
tunatap connect my-cluster

# Run kubectl commands through the tunnel
tunatap exec my-cluster -- kubectl get nodes

# Speed up discovery with a region hint
tunatap connect my-cluster --region us-phoenix-1
```

Tunatap will:
1. Search all compartments across all subscribed regions
2. Find the cluster and its bastion
3. Generate ephemeral SSH keys
4. Establish the tunnel

Results are cached for 24 hours for fast subsequent connections.

### Traditional Mode (with config file)

If you prefer explicit configuration:

1. Initialize configuration:
   ```bash
   tunatap setup init
   ```

2. Run the interactive setup wizard:
   ```bash
   tunatap setup
   ```

3. Connect to a cluster:
   ```bash
   tunatap connect my-cluster
   ```

## Configuration

Configuration is stored in `~/.tunatap/config.yaml`.

### Example Configuration

```yaml
ssh_private_key_file: ~/.ssh/id_rsa
ssh_socks_proxy: ""
ssh_connection_pool_size: 5
ssh_connection_warmup_count: 2
ssh_connection_max_concurrent_use: 10

tenancies:
  my-tenancy: ocid1.tenancy.oc1..example

clusters:
  - cluster_name: prod-cluster
    region: us-ashburn-1
    tenant: my-tenancy
    compartment: infrastructure/kubernetes
    bastion: prod-bastion
    local_port: 6443
    endpoints:
      - name: private
        ip: 10.0.1.100
        port: 6443
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `ssh_private_key_file` | Path to SSH private key | `~/.ssh/id_rsa` |
| `ssh_socks_proxy` | SOCKS proxy address (optional) | - |
| `ssh_connection_pool_size` | Max SSH connections in pool | 5 |
| `ssh_connection_warmup_count` | Connections to pre-establish | 2 |
| `ssh_connection_max_concurrent_use` | Max concurrent uses per connection | 10 |
| `oci_auth_type` | Authentication method: `auto`, `config`, `instance_principal`, `resource_principal`, `security_token` | `auto` |
| `oci_config_path` | Path to OCI config file | `~/.oci/config` |
| `oci_profile` | OCI config profile name | `DEFAULT` |
| `use_ephemeral_keys` | Use in-memory SSH keys instead of file-based | `false` |
| `cache_ttl_hours` | Discovery cache time-to-live in hours | `24` |
| `skip_discovery` | Disable automatic cluster discovery | `false` |
| `discovery_regions` | Regions to search during discovery (empty = all subscribed) | `[]` |
| `health_endpoint` | Address for health HTTP server (e.g., `localhost:9090`) | - |

## Commands

### connect

Connect to a cluster through bastion. Works in both zero-touch and config-based modes.

```bash
tunatap connect [cluster-name]

# Flags
-c, --cluster    Cluster name to connect to
-p, --port       Local port for the tunnel (0 for auto)
-b, --bastion    Bastion name to use
-e, --endpoint   Endpoint name (e.g., 'private', 'public')
-r, --region     Region hint for discovery (speeds up search)
    --no-bastion Connect directly without bastion
    --no-cache   Skip cache and force fresh discovery
    --preflight  Run preflight checks before connecting
```

### exec

Run a command with tunnel and kubeconfig automatically configured.

```bash
tunatap exec [cluster] -- <command> [args...]

# Examples
tunatap exec my-cluster -- kubectl get nodes
tunatap exec my-cluster -- helm list -A
tunatap exec -c prod -- k9s

# Flags
-c, --cluster      Cluster name to connect to
-e, --endpoint     Endpoint name (e.g., 'private', 'public')
-b, --bastion      Bastion name to use
-r, --region       Region hint for discovery
    --no-oci-auth  Disable OCI exec-auth in kubeconfig
    --oci-profile  OCI config profile for exec-auth
    --no-cache     Skip cache and force fresh discovery
```

The exec command:
1. Establishes a tunnel to the cluster
2. Creates a temporary kubeconfig pointing to `localhost:<port>`
3. Sets `KUBECONFIG` environment variable
4. Runs your command
5. Cleans up tunnel and kubeconfig on exit

### cache

Manage the discovery cache.

```bash
# Show all cached entries
tunatap cache show

# Clear entire cache
tunatap cache clear

# Clear cache for a specific cluster
tunatap cache clear my-cluster
```

### setup

Interactive configuration wizard.

```bash
tunatap setup           # Run full setup wizard
tunatap setup init      # Initialize new config file
tunatap setup show      # Show current configuration
tunatap setup add-cluster    # Add a cluster interactively
tunatap setup add-tenancy <name> <ocid>  # Add a tenancy
```

### list

List configured resources.

```bash
tunatap list clusters   # List configured clusters
tunatap list bastions   # List bastions in a compartment
```

### doctor

Diagnose configuration and connectivity issues.

```bash
tunatap doctor          # Run diagnostics
tunatap doctor -v       # Verbose output with connectivity test
```

### catalog

Manage cluster catalogs from remote sources.

```bash
tunatap catalog sync    # Sync clusters from catalog sources
tunatap catalog list    # List catalog sources
```

### audit

Audit configuration and access patterns.

```bash
tunatap audit           # Run configuration audit
```

### status

Show active tunnel status.

```bash
tunatap status          # Show active tunnels
tunatap status --json   # Output as JSON
tunatap status -v       # Verbose output with session details
```

### logs

View tunnel activity logs.

```bash
tunatap logs            # Show recent activity (last 20 entries)
tunatap logs -f         # Follow/stream new log entries (like tail -f)
tunatap logs -n 50      # Show last 50 entries
tunatap logs -c prod    # Filter by cluster name
tunatap logs --json     # Output raw JSON entries
```

### version

Print version information.

```bash
tunatap version
```

## Global Flags

```bash
--config    Config file path (default: ~/.tunatap/config.yaml)
--debug     Enable debug logging
--raw       Output raw logs to file instead of console
```

## Usage with kubectl

Once connected, use kubectl in another terminal:

```bash
# The tunnel forwards localhost:6443 to your cluster
kubectl --server=https://localhost:6443 get nodes

# Or configure your kubeconfig to use the tunnel
kubectl config set-cluster my-cluster --server=https://localhost:6443
```

## Health Endpoint

When configured, tunatap exposes HTTP endpoints for monitoring:

```yaml
# In ~/.tunatap/config.yaml
health_endpoint: "localhost:9090"
```

**Security:** The health endpoint is restricted to localhost by default. Non-localhost addresses are automatically rewritten to `127.0.0.1` to prevent accidental network exposure. Sensitive data (session IDs, internal IPs) is redacted from responses.

Available endpoints:

| Endpoint | Description |
|----------|-------------|
| `/health` | JSON status of tunnels (sensitive data redacted) |
| `/healthz` | Simple `ok` for Kubernetes liveness probes |
| `/readyz` | Returns `ok` when healthy tunnels exist (readiness) |
| `/metrics` | Prometheus-format metrics (no infrastructure names) |

Example metrics:
```
tunatap_up 1
tunatap_uptime_seconds 3600
tunatap_tunnels_total 2
tunatap_tunnel_healthy{tunnel="0",local_port="6443"} 1
tunatap_pool_size{tunnel="0",local_port="6443"} 5
tunatap_pool_active_uses{tunnel="0",local_port="6443"} 2
```

## Troubleshooting

Run the doctor command to diagnose issues:

```bash
tunatap doctor -v
```

Common issues:

1. **OCI config not found**: Run `oci setup config` to configure OCI CLI
2. **SSH key not found**: Ensure your SSH key exists at the configured path
3. **Bastion session fails**: Check your OCI permissions for Bastion service
4. **Connection refused**: Verify the cluster endpoint IP and port

## Versioning

Tunatap uses [Calendar Versioning (CalVer)](https://calver.org/) with the format `YYYY.MM.BUILD`:

- `2026.01.1` - First release of January 2026
- `2026.01.2` - Second release of January 2026
- `2026.02.1` - First release of February 2026

This versioning scheme makes it easy to understand when a release was made.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes and ensure tests pass (`go test -race ./...`)
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

**Note:** All changes require a Pull Request - direct pushes to main are not allowed.

### Releasing

Releases are automated via GitHub Actions using [Calendar Versioning](https://calver.org/). To trigger a release:
- Include `[release]` in your PR title or commit message when merging to main
- Or manually trigger the Release workflow from the Actions tab

## License

MIT
