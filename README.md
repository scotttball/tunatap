# Tunatap

```
            *  . *
        . *    SPLASH!   *
     *    \   |   /    .
          .-""""""-.
        .'  ___     '.         ~  ~  ~
       /  .'   '.  @  \       ~       ~
      |  /  ___  \     |    ~    ~  ~
     /| |  (   )  |    |\      ~     ~
    | | |   '-'   | |\ | |   ~   ~ ~
    | \ \  \___/  / |  \| |     ~  ~
     \  '.       .'  \    /   ~    ~
      \   '-...-'  _.-\  /
       '.   """"   .' \/         ><((('>
         '-......-'      ~
                     ~        ~   ~
      ~~~~~~~~~~~~~~~~~~~~~~~~~~~~
          T U N A T A P
       SSH Tunnel Manager
```

SSH tunnel manager for OCI Bastion services. Simplifies connecting to private OKE (Oracle Kubernetes Engine) clusters and other private resources through OCI Bastion hosts.

## Features

- **SSH Tunneling**: Establish secure tunnels through OCI Bastion services
- **Connection Pooling**: Efficient SSH connection reuse with configurable pool size
- **Bastion Types**: Support for both STANDARD (OCI Bastion service) and INTERNAL (jump box) modes
- **Interactive Selection**: Fuzzy finder (fzf) for cluster selection
- **SOCKS Proxy**: Route SSH connections through SOCKS proxies
- **Cross-Platform**: Full support for Linux, macOS, and Windows
- **Multiple Auth Methods**: OCI config file, instance principal, resource principal, security token, auto-detect
- **Kubeconfig Injection**: Automatic kubeconfig generation for connected clusters
- **Remote Config**: Load shared cluster catalogs from OCI Object Storage
- **Health Monitoring**: Automatic connection health checks with keepalive probes
- **Session Management**: Automatic bastion session refresh before expiration
- **Preflight Checks**: Built-in diagnostics for troubleshooting connectivity

## Installation

### From Source

```bash
git clone https://github.com/scotttball/tunatap.git
cd tunatap
go build -o tunatap .
```

### Cross-Platform Builds

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o tunatap-linux .

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o tunatap-darwin .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o tunatap-darwin-arm64 .

# Windows
GOOS=windows GOARCH=amd64 go build -o tunatap.exe .
```

## Prerequisites

- OCI CLI configured (`~/.oci/config`)
- SSH key pair for bastion authentication
- Access to OCI Bastion service in your tenancy

## Quick Start

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

## Commands

### connect

Connect to a cluster through bastion.

```bash
tunatap connect [cluster-name]

# Flags
-c, --cluster    Cluster name to connect to
-p, --port       Local port for the tunnel (default: 6443)
-b, --bastion    Bastion name to use
-e, --endpoint   Endpoint name (e.g., 'private', 'public')
    --no-bastion Connect directly without bastion
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

## License

MIT
