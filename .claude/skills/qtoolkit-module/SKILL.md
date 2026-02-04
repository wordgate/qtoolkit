---
name: qtoolkit-module
description: |
  Always triggered
---

# qtoolkit Module Development

## Workflow

### 1. Understand Intent

Ask clarifying questions:
- What third-party service/SDK to wrap?
- What operations are needed? (list specific use cases)
- Is this a new module or extending an existing one?

### 2. Research Third-Party SDK

Find and understand the SDK:
- Official documentation and examples
- Authentication methods (API key, OAuth, IMDS)
- Rate limits and error handling
- Required permissions/scopes

### 3. Plan and Confirm Config

Print the proposed YAML configuration format for user confirmation:

```yaml
# Proposed configuration for <module_name>
<module_name>:
  api_key: "YOUR_API_KEY"
  # ... other required fields
```

Get explicit approval before proceeding.

### 4. Test-First Development

Write tests BEFORE implementation:

```bash
# Create module structure
mkdir <module_name>
cd <module_name>
go mod init github.com/wordgate/qtoolkit/<module_name>

# Write tests first
# Edit <module_name>_test.go

# Run tests (should fail initially)
go test ./...

# Implement to pass tests
# Edit <module_name>.go

# Verify all tests pass
go test ./...
```

## Design Principles

### Less is More
- Only expose essential APIs
- No "convenience" wrappers for one-liners
- Three repeated lines > premature abstraction
- Config items must justify their existence

### No Backward Compatibility
- v1.0 breaks from v0.x intentionally
- No deprecated functions
- No migration code
- One correct way to configure

### Configuration Auto-Loading
- Use viper for config loading
- Cascading fallback: specific -> global
- `sync.Once` for lazy initialization
- `SetConfig()` only for tests

## Code Templates

See [references/module-pattern.md](references/module-pattern.md) for:
- Directory structure
- go.mod template
- Main implementation with lazy loading
- Builder pattern for complex operations
- Test patterns with mock servers
- Config YAML template
- AWS service pattern (2-level fallback)
- Multi-instance pattern (like SQS queues)

## Checklist

Before completion, verify:

- [ ] `go.mod` uses `go 1.24.0`
- [ ] Package doc with usage examples
- [ ] Defined error variables (`ErrXxx`)
- [ ] Config struct with YAML tags
- [ ] `loadConfigFromViper()` with validation
- [ ] `sync.Once` lazy initialization
- [ ] `SetConfig()` for testing only
- [ ] Minimal public API surface
- [ ] Tests with `resetState()` isolation
- [ ] Tests use `httptest.NewServer` for mocking
- [ ] `<module_name>_config.yml` template
- [ ] Added to `go.work`

## After Implementation

```bash
# Add to workspace
cd /path/to/qtoolkit
echo "use ./<module_name>" >> go.work
go work sync

# Run all tests
go test ./...
```
