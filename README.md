# enver

> **Disclaimer:** This entire codebase was vibe coded with AI assistance. Use at your own risk. It probably works, but no guarantees were made and certainly none were kept.

A CLI tool to generate `.env` files from a kubernetes cluster, to use for development.

## Installation

Download the version for your platform from the releases page.

## Commands

### generate

Generate a single `.env` file interactively or with flags from configuration in the configuration file `.enver.yaml`.

```bash
enver generate [flags]
```

#### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output-name` | | `.env` | Output file name |
| `--output-directory` | | `generated` | Output directory for the .env file |
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

  # Define variables inline
  - type: Vars
    name: inline-vars  # optional, used in output comments
    vars:
      - name: APP_ENV
        value: production
      - name: LOG_LEVEL
        value: info
```

### Source Types

| Type | Description | Required Fields |
|------|-------------|-----------------|
| `ConfigMap` | Kubernetes ConfigMap | `name` |
| `Secret` | Kubernetes Secret | `name` |
| `Deployment` | Kubernetes Deployment env vars | `name` |
| `StatefulSet` | Kubernetes StatefulSet env vars | `name` |
| `DaemonSet` | Kubernetes DaemonSet env vars | `name` |
| `EnvFile` | Local .env file | `path` |
| `Vars` | Inline variables | `vars` |

### Deployment, StatefulSet, and DaemonSet Sources

The `Deployment`, `StatefulSet`, and `DaemonSet` sources extract environment variables from the respective Kubernetes workload's container specifications:

```yaml
sources:
  - type: Deployment      # or StatefulSet, DaemonSet
    name: my-app
    namespace: default    # optional, defaults to "default"
    containers:           # optional, defaults to all containers
      - app
      - sidecar
```

All three source types retrieve:
- `env` entries with direct `value`
- `env` entries with `valueFrom` (ConfigMapKeyRef, SecretKeyRef)
- `envFrom` entries with `configMapRef` (all keys from the ConfigMap)
- `envFrom` entries with `secretRef` (all keys from the Secret)
- `volumeMounts` referencing ConfigMap or Secret volumes (including projected volumes)

For volume mounts, the `file` transformation is automatically applied to write each key's content to a file at the mount path. The environment variable will contain the file path.

#### Volume Mount Key Mappings

By default, volume mount keys from ConfigMaps and Secrets are used as the environment variable names. You can customize this with `volumeMountKeyMappings`:

```yaml
sources:
  - type: Deployment
    name: my-app
    volumeMountKeyMappings:
      - kind: ConfigMap
        name: app-config
        mappings:
          config.yaml: APP_CONFIG_FILE
          settings.json: APP_SETTINGS_FILE
      - kind: Secret
        name: app-certs
        mappings:
          tls.crt: TLS_CERT_PATH
          tls.key: TLS_KEY_PATH
```

This maps the original key names from the ConfigMap/Secret to custom environment variable names.

**Note:** Field references (`fieldRef`) and resource field references (`resourceFieldRef`) are skipped as they require pod runtime context.

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

You can exclude specific environment variables from a source using exact names or regex patterns:

```yaml
sources:
  - type: ConfigMap
    name: my-config
    variables:
      exclude:
        - DEBUG           # exact match
        - INTERNAL_KEY    # exact match
        - ^TEMP_.*        # regex: exclude all vars starting with TEMP_

  - type: Secret
    name: app-secrets
    variables:
      exclude:
        - TEMP_TOKEN
        - .*_SECRET$      # regex: exclude all vars ending with _SECRET

  - type: EnvFile
    path: ./local.env
    variables:
      exclude:
        - LOCAL_ONLY_VAR
        - ^(DEBUG|TRACE)_ # regex: exclude DEBUG_* and TRACE_* vars
```

Patterns in `exclude` are first matched exactly, then as regex patterns. Variables matching any pattern will be filtered out and not included in the generated `.env` file.

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

| Type | Description | Target | Additional Fields |
|------|-------------|--------|-------------------|
| `base64_decode` | Decode base64 encoded string | `key` or `value` | - |
| `base64_encode` | Encode string to base64 | `key` or `value` | - |
| `prefix` | Add prefix to string | `key` or `value` | `value` |
| `suffix` | Add suffix to string | `key` or `value` | `value` |
| `absolute_path` | Convert relative path to absolute path | `value` only | - |
| `output_directory` | Set value to the output directory | `value` only | - |
| `file` | Write value to file, replace with file path | `value` only | `output`, `key` |

#### Transformation Fields

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Transformation type (see table above) |
| `target` | For most types | What to transform: `key` or `value` |
| `value` | For prefix/suffix | The string to add |
| `variables` | No | Limit to specific variable names (empty = apply to all) |
| `output` | For file | Output file path to write the value to (relative paths are resolved against output directory) |
| `key` | For file | New environment variable name for the file path |

#### File Transformation Example

The `file` transformation writes the variable's value to a file and replaces the value with the file path:

```yaml
sources:
  - type: Secret
    name: my-certs
    transformations:
      - type: file
        output: cert.pem
        key: CERT_FILE_PATH
        variables:
          - CERTIFICATE
```

This will:
1. Take the value of `CERTIFICATE` from the secret
2. Write it to `<output-directory>/cert.pem` (e.g., `generated/cert.pem`)
3. Output `CERT_FILE_PATH=generated/cert.pem` in the .env file

Relative paths in `output` are resolved against the output directory. Use absolute paths if you need to write files elsewhere.

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
    output:
      name: local.env
      directory: ./generated
    contexts:
      - local

  - name: development
    output:
      name: dev.env
      directory: ./generated
    kube-context: dev-cluster
    contexts:
      - development

  - name: production
    output:
      name: prod.env
      directory: ./generated
    kube-context: prod-cluster
    contexts:
      - production
```

#### Execution Fields

| Field | Default | Description |
|-------|---------|-------------|
| `name` | | Identifier for the execution (displayed during execution) |
| `output.name` | `.env` | File name for the generated .env file |
| `output.directory` | `generated` | Directory for the generated .env file |
| `contexts` | | List of contexts to filter sources |
| `kube-context` | | Kubernetes context to use (required if execution uses ConfigMap or Secret sources) |

## Examples

### Basic usage

Generate a `.env` file with interactive prompts:

```bash
enver generate
```

### Specify output location

```bash
enver generate --output-name .env.local
enver generate --output-directory ./config
enver generate --output-name prod.env --output-directory ./config
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
enver generate -c production --kube-context prod-cluster --output-name .env.production --output-directory .
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

The generated `.env` file includes comments showing the source, with variables grouped by source:

```bash
# ConfigMap default/my-app-config
DATABASE_HOST=localhost
DATABASE_PORT=5432

# Secret production/my-app-secrets
DATABASE_PASSWORD=secret123

# EnvFile ./local.env
DEBUG=true
```

## Gitignore Protection

When running inside a git repository, enver checks if generated files are covered by `.gitignore`. This applies to:

- Output `.env` files from `generate` and `execute` commands
- Files created by the `file` transformation

If a file is not gitignored, you'll be prompted with options:

1. **Add file**: Add the specific file path to `.gitignore`
2. **Add directory**: Add the file's directory to `.gitignore` (with trailing `/`)
3. **Skip**: Do nothing

This helps prevent accidentally committing sensitive environment files or secrets to version control.

## IDE Integration

A JSON schema is provided for `.enver.yaml` validation and autocompletion.

### VS Code

Add the following to your `.vscode/settings.json` or global settings:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/kroonprins/enver/main/enver.schema.json": ".enver.yaml"
  }
}
```

**Note:** Requires the [YAML extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml) by Red Hat.

### IntelliJ IDEA

1. Open **Settings/Preferences** > **Languages & Frameworks** > **Schemas and DTDs** > **JSON Schema Mappings**
2. Click **+** to add a new mapping
3. Set **Name** to `enver`
4. Set **Schema file or URL** to:
   - Local: path to `enver.schema.json`
   - Remote: `https://raw.githubusercontent.com/kroonprins/enver/main/enver.schema.json`
5. Add a file pattern: `.enver.yaml`
6. Click **OK**

Alternatively, add a schema reference directly in your `.enver.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/kroonprins/enver/main/enver.schema.json

contexts:
  - local
  - production
# ...
```

## Development

### Running Tests

```bash
# Run unit tests
make test-unit

# Run E2E tests (requires Kind cluster)
make setup-kind      # Create Kind cluster if needed
make test-e2e        # Run E2E tests

# Update golden files after intentional changes
make test-e2e-update
```

E2E tests use a Kind cluster named `kind` and create a temporary namespace `enver-e2e-test` for test resources.

## Interactive Prompts

When flags are not provided:

1. **Context selection**: If `contexts` are defined in `.enver.yaml`, you'll be prompted to select one or more contexts (or none)
2. **Kubernetes context selection**: If any ConfigMap or Secret sources will be processed, you'll be prompted to select a kubectl context from your kubeconfig
