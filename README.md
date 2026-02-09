# enver

A CLI tool that generates `.env` files from Kubernetes ConfigMaps, Secrets, and local env files.

## Installation

```bash
go build -o enver
```

## Commands

### generate

Generate a single `.env` file interactively or with flags.

```bash
enver generate [flags]
```

#### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `generated/.env` | Output file path for the .env file |
| `--context` | `-c` | | Context for filtering sources (can be repeated) |
| `--kube-context` | | | Kubernetes context to use |

### execute

Execute predefined generation tasks from `.enver.yaml`.

```bash
enver execute [flags]
```

#### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--all` | | `false` | Run all executions |
| `--name` | | | Execution name to run (can be repeated) |

If neither `--all` nor `--name` is provided, you'll be prompted to select which executions to run.

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

### Variable Exclusion

You can exclude specific environment variables from a source:

```yaml
sources:
  - type: ConfigMap
    name: my-config
    variables:
      exclude:
        - DEBUG
        - INTERNAL_KEY

  - type: Secret
    name: app-secrets
    variables:
      exclude:
        - TEMP_TOKEN

  - type: EnvFile
    path: ./local.env
    variables:
      exclude:
        - LOCAL_ONLY_VAR
```

Variables listed in `exclude` will be filtered out and not included in the generated `.env` file.

### Transformations

You can apply transformations to variable keys or values:

```yaml
sources:
  - type: ConfigMap
    name: my-config
    transformations:
      # Add prefix to all variable keys
      - type: prefix
        target: key
        value: "APP_"

      # Decode base64 values for specific variables
      - type: base64_decode
        target: value
        variables:
          - ENCODED_SECRET
          - ENCODED_TOKEN

      # Add suffix to specific variable values
      - type: suffix
        target: value
        value: "_prod"
        variables:
          - DATABASE_HOST
```

#### Available Transformations

| Type | Description | Requires `value` |
|------|-------------|------------------|
| `base64_decode` | Decode base64 encoded string | No |
| `base64_encode` | Encode string to base64 | No |
| `prefix` | Add prefix to string | Yes |
| `suffix` | Add suffix to string | Yes |

#### Transformation Fields

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Transformation type (see table above) |
| `target` | Yes | What to transform: `key` or `value` |
| `value` | For prefix/suffix | The string to add |
| `variables` | No | Limit to specific variable names (empty = apply to all) |

Transformations are applied in order as configured.

### Executions

Define predefined generation tasks that can be run with `enver execute`:

```yaml
contexts:
  - local
  - development
  - production

sources:
  - type: EnvFile
    path: ./local.env
    contexts:
      include:
        - local
  - type: ConfigMap
    name: app-config
    contexts:
      include:
        - development
        - production
  - type: Secret
    name: app-secrets
    contexts:
      include:
        - production

executions:
  - name: local
    output: ./generated/local.env
    contexts:
      - local

  - name: development
    output: ./generated/dev.env
    kube-context: dev-cluster
    contexts:
      - development

  - name: production
    output: ./generated/prod.env
    kube-context: prod-cluster
    contexts:
      - production
```

#### Execution Fields

| Field | Description |
|-------|-------------|
| `name` | Identifier for the execution (displayed during execution) |
| `output` | Path for the generated .env file |
| `contexts` | List of contexts to filter sources |
| `kube-context` | Kubernetes context to use (optional, prompts if not specified) |

The `kube-context` priority is:
1. Execution's `kube-context` field
2. Interactive prompt (if not specified)

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

### Execute predefined tasks

Run all executions:

```bash
enver execute --all
```

Run specific executions by name:

```bash
enver execute --name local
enver execute --name local --name production
```

Interactive selection (prompts to choose executions):

```bash
enver execute
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
