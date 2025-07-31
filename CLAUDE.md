# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
qtoolkit stands for "Quality Toolkit"
This is a Go toolkit library for the WordGate platform, organized as a modular monorepo with various service integrations. 

## Development Commands

### Testing
- Run tests: `go test ./...`
- Run specific tests: `go test ./exchange_rate_api_test.go ./slack_test.go`

### Building
- Build the main package: `go build`
- Build with module replacement: The project uses local module replacements in go.mod for its submodules

### Module Management
- The project uses Go modules with local replacements for internal packages
- Key modules include: log, appstore, wordgate (including SDK), util
- Run `go mod tidy` after making changes to dependencies

## Architecture

### Core Structure
The project is organized with the main qtoolkit package at the root, containing:

1. **Service Integrations**: Cloud provider integrations (AWS, Aliyun), third-party services (GoDaddy, Slack, App Store)
2. **WordGate Client**: Core API client for WordGate platform interactions (wordgate.go)
3. **Utility Modules**: Shared utilities, logging, database operations
4. **Event System**: Event-driven architecture with retry mechanisms (event.go)
5. **Configuration Management**: Viper-based configuration with YAML support (config.go)

### Key Components

#### WordGate Integration (`wordgate.go`)
- HTTP client for WordGate API communication
- Order management (create, retrieve)
- Product management (CRUD operations, bulk sync)
- Authentication via app code/secret headers

#### WordGate SDK (`wordgate/sdk/`)
- Higher-level client wrapper with configuration management
- Supports syncing products, memberships, and app configuration
- Content processing from Markdown files
- Dry-run capabilities for testing

#### Event System (`event.go`)
- Global event dispatcher with prefix-based subscriptions
- Automatic retry mechanism with configurable parameters
- Context-aware logging integration

#### Utility Modules
- `util/`: Common utilities (time, UUID, validation, image processing)
- `log/`: Structured logging with CloudWatch integration
- Database: GORM-based MySQL integration

### Module Dependencies
The project uses local module replacements for internal packages:
- `github.com/wordgate/qtoolkit/log` → `./log`
- `github.com/wordgate/qtoolkit/appstore` → `./appstore`
- `github.com/wordgate/qtoolkit/wordgate` → `./wordgate`
- `github.com/wordgate/qtoolkit/wordgate/sdk` → `./wordgate/sdk`

## Configuration

### Configuration Files
All configuration files use YAML format with placeholder values for sensitive data:
- `wordgate.config.yml`: Main WordGate configuration
- `aws_config.yml`, `aliyun_config.yml`: Cloud provider settings
- `mail_config.yml`, `slack_config.yml`: Service-specific configurations

### Environment Setup
- Use `SetConfigFile(file string)` to initialize configuration
- Supports development/test modes via `is_dev` and `is_test` flags
- Automatically sets Gin mode based on environment

## Security Considerations

The README.md contains important security guidance:
- Never commit real credentials to version control
- Use environment variables for production deployments
- Rotate credentials regularly
- Use least-privilege access for API keys

Replace placeholder values in configuration files before use:
- `YOUR_AWS_ACCESS_KEY`, `YOUR_AWS_SECRET_KEY` in AWS configs
- `YOUR_ALIYUN_ACCESS_KEY`, `YOUR_ALIYUN_ACCESS_SECRET` in Aliyun configs
- Various API keys and secrets in service-specific configurations