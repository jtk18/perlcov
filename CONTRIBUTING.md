# Contributing to perlcov

Thank you for your interest in contributing to perlcov! This document provides guidelines and instructions for development.

## Prerequisites

### Required Software

- **Go 1.21+**: Download from [golang.org](https://golang.org/dl/)
- **Perl 5.10+**: Usually pre-installed on Unix systems
- **Devel::Cover**: Install via CPAN

```bash
# Install Devel::Cover
cpan Devel::Cover

# Or using cpanm
cpanm Devel::Cover
```

### Verifying Installation

```bash
# Check Go version
go version

# Check Perl version
perl -v

# Check Devel::Cover
perl -MDevel::Cover -e 'print "Devel::Cover $Devel::Cover::VERSION\n"'
```

## Getting Started

### Clone the Repository

```bash
git clone https://github.com/user/perlcov.git
cd perlcov
```

### Build the Project

```bash
# Build the binary
go build -o perlcov ./cmd/perlcov/

# Run tests
go test ./...

# Run with race detection (recommended during development)
go build -race -o perlcov ./cmd/perlcov/
```

### Project Structure

```
perlcov/
├── cmd/
│   └── perlcov/
│       └── main.go           # Entry point
├── internal/
│   ├── cli/
│   │   └── cli.go            # Command-line interface and flags
│   ├── coverage/
│   │   └── coverage.go       # Coverage parsing and reporting
│   └── runner/
│       └── runner.go         # Test execution with Devel::Cover
├── go.mod
├── README.md
├── CONTRIBUTING.md
└── LICENSE
```

## Development Workflow

### Running Locally

```bash
# Build and run
go build -o perlcov ./cmd/perlcov/ && ./perlcov --help

# Test against a Perl project
cd /path/to/perl-project
/path/to/perlcov -I lib -j 4
```

### Testing with a Sample Project

```bash
# Clone Moo as a test project
git clone --depth 1 https://github.com/moose/Moo.git /tmp/moo-test

# Run perlcov against it
cd /tmp/moo-test
/path/to/perlcov -I lib -j 4
```

### Adding Features

1. **Create a branch** for your feature
2. **Write tests** if applicable
3. **Update documentation** in README.md
4. **Test manually** against a real Perl project
5. **Submit a pull request**

## Code Style

### Go Code

- Follow standard Go conventions (use `gofmt`)
- Use meaningful variable names
- Add comments for exported functions
- Keep functions focused and small

```bash
# Format code
go fmt ./...

# Lint code (install golangci-lint first)
golangci-lint run
```

### Embedded Perl Scripts

The coverage parsing uses embedded Perl scripts. When modifying these:

- Keep scripts minimal and focused
- Use `strict` and `warnings`
- Handle edge cases gracefully
- Test with different Devel::Cover database formats

## Testing

### Unit Tests

```bash
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...
```

### Integration Testing

Test against real Perl projects:

```bash
# Small project (fast)
git clone --depth 1 https://github.com/moose/Moo.git /tmp/moo
cd /tmp/moo && perlcov -I lib

# Larger project (for performance testing)
git clone --depth 1 https://github.com/moose/Moose.git /tmp/moose
cd /tmp/moose && perlcov -I lib -j 8
```

## Common Issues

### "Devel::Cover is not installed"

Install Devel::Cover:
```bash
cpan Devel::Cover
```

### Coverage shows 0% despite tests running

- Ensure `-I lib` includes the correct source directories
- Check that the `--source` flag points to your Perl modules
- Verify that tests actually load and exercise the modules

### Parallel tests causing issues

Some tests may not be parallelization-safe. Try reducing `-j` or running sequentially:
```bash
perlcov -j 1
```

## Reporting Bugs

When reporting bugs, please include:

1. **perlcov version**: `perlcov --version`
2. **Go version**: `go version`
3. **Perl version**: `perl -v`
4. **Devel::Cover version**: `perl -MDevel::Cover -e 'print $Devel::Cover::VERSION'`
5. **Operating system**
6. **Minimal reproduction steps**
7. **Expected vs actual behavior**

## Feature Requests

We welcome feature requests! Please open an issue describing:

1. The problem you're trying to solve
2. Your proposed solution
3. Any alternatives you've considered

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Commit with clear messages (`git commit -m 'Add amazing feature'`)
6. Push to your fork (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## License

By contributing to perlcov, you agree that your contributions will be licensed under the MIT License.
