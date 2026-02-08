# enver

A CLI tool that generates `.env` files from Kubernetes ConfigMaps, Secrets, and local env files.

## Installation

```bash
go build -o enver
```

## Usage

```bash
enver generate [flags]
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `generated/.env` | Output file path for the .env file |
| `--context` | `-c` | | Context for filtering sources (can be repeated) |
| `--kube-context` | | | Kubernetes context to use |

## Configuration

Create a `.enver.yaml` file in your project root:

```yaml
# Optional: Define contexts for filtering sources
contexts:
  - local
  - development
  - production

# Define your sources
sources:
  # Read from a Kubernetes ConfigMap
  - type: ConfigMap
    name: my-app-config
    namespace: default  # optional, defaults to "default"

  # Read from a Kubernetes Secret
  - type: Secret
    name: my-app-secrets
    namespace: production

  # Read from a local .env file
  - type: EnvFile
    path: ./local.env  # supports absolute or relative paths
```

### Source Types

| Type | Description | Required Fields |
|------|-------------|-----------------|
| `ConfigMap` | Kubernetes ConfigMap | `name` |
| `Secret` | Kubernetes Secret | `name` |
| `EnvFile` | Local .env file | `path` |

### Context Filtering

You can filter which sources are included based on contexts:

```yaml
contexts:
  - local
  - development
  - production

sources:
  # Only included when "local" context is selected
  - type: EnvFile
    path: ./local.env
    contexts:
      include:
        - local

  # Included in all contexts except "production"
  - type: ConfigMap
    name: debug-config
    contexts:
      exclude:
        - production

  # Always included (no context restrictions)
  - type: ConfigMap
    name: shared-config
```

## Examples

### Basic usage

Generate a `.env` file with interactive prompts:

```bash
enver generate
```

### Specify output file

```bash
enver generate -o .env
enver generate --output ./config/.env
```

### Specify contexts

```bash
# Single context
enver generate -c development

# Multiple contexts
enver generate -c development -c staging
```

### Specify Kubernetes context

```bash
enver generate --kube-context kind-kind
```

### Full example

```bash
enver generate -c production --kube-context prod-cluster -o .env.production
```

## Output Format

The generated `.env` file includes comments showing the source of each variable:

```bash
# ConfigMap default/my-app-config
DATABASE_HOST=localhost
# ConfigMap default/my-app-config
DATABASE_PORT=5432
# Secret production/my-app-secrets
DATABASE_PASSWORD=secret123
# EnvFile ./local.env
DEBUG=true
```

## Interactive Prompts

When flags are not provided:

1. **Context selection**: If `contexts` are defined in `.enver.yaml`, you'll be prompted to select one or more contexts (or none)
2. **Kubernetes context selection**: If any ConfigMap or Secret sources will be processed, you'll be prompted to select a kubectl context from your kubeconfig
