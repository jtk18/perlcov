# perlcov

A fast, parallel Perl test coverage tool that wraps [Devel::Cover](https://metacpan.org/pod/Devel::Cover).

## Features

- **Parallel test execution** - Run tests concurrently for significant speedups
- **Perly interface** - Familiar `-I` flag for include paths
- **Clean coverage reports** - Readable text output showing statement, branch, condition, and subroutine coverage
- **Failed test detection** - Identify whether test failures are caused by Devel::Cover itself
- **HTML reports** - Optional HTML generation via Devel::Cover's `cover` command

## Performance

Benchmarked against [Moo](https://github.com/moose/Moo) (71 test files, 841 tests) on GitHub Actions (4 cores):

| Method | Time | Speedup |
|--------|------|---------|
| Baseline (no coverage) | 4.7s | - |
| Sequential Devel::Cover + Storable | 37.3s | 1x |
| Sequential Devel::Cover + JSON::PP | 46.8s | 0.8x (slowest) |
| Sequential Devel::Cover + JSON::XS | 36.9s | 1x |
| Sequential Devel::Cover + Sereal | 37.1s | 1x |
| **perlcov -j 4** | **14.8s** | **2.5x faster** |

Key findings:
- **perlcov provides 2.5x speedup** over sequential Devel::Cover
- **JSON::PP is significantly slower** than other formats (use `Cpanel::JSON::XS` or `Sereal`)
- Storable, JSON::XS, and Sereal have similar performance

### Performance Tips

Avoid using `JSON::MaybeXS` without `Cpanel::JSON::XS` installed, as pure-Perl JSON::PP is ~25% slower than Storable. For best performance, install one of:

```bash
cpan Cpanel::JSON::XS   # Fast JSON (recommended)
cpan Sereal             # Fast binary format
```

When JSON::MaybeXS with an XS backend is installed, perlcov automatically detects JSON-formatted coverage data and uses pure Go parsing for the merge step.

### Run Benchmarks Yourself

A Dockerfile is provided to run reproducible benchmarks in a clean environment:

```bash
docker build -f Dockerfile.benchmark -t perlcov-benchmark .
docker run --rm perlcov-benchmark
```

This runs the full benchmark suite against the Moo test suite, testing Storable, JSON::PP, JSON::XS, and Sereal formats.

### Note on HTML Reports

Generating HTML reports via the `--html` flag uses the `cover` command, which can be slow for large codebases. For 1000+ test files, HTML generation may take several minutes. The text report is generated instantly using direct parallel parsing of the coverage database.

## Installation

### Prerequisites

- Go 1.21 or later
- Perl 5.10 or later
- Devel::Cover (`cpan Devel::Cover`)

### Building from Source

```bash
git clone https://github.com/jtk18/perlcov.git
cd perlcov
go build -o perlcov ./cmd/perlcov/

# Optionally install to your PATH
sudo mv perlcov /usr/local/bin/
```

## Usage

```bash
# Run all tests in t/**/*.t with coverage
perlcov

# Run with 4 parallel jobs
perlcov -j 4

# Add include paths (like Perl's -I flag)
perlcov -I lib -I local/lib/perl5

# Generate HTML report (warning: can be slow for large projects)
perlcov --html

# Run specific test files or directories
perlcov t/unit/
perlcov t/foo.t t/bar.t

# Disable automatic rerun of failed tests (enabled by default)
perlcov --no-rerun-failed

# Verbose output with uncovered line numbers
perlcov -v
```

### Options

| Flag | Description |
|------|-------------|
| `-I <path>` | Add directory to @INC (can be specified multiple times) |
| `-j <n>` | Number of parallel test jobs (default: all CPUs) |
| `--html` | Generate HTML coverage report (slow for large projects) |
| `--cover-dir <dir>` | Directory for coverage database (default: `cover_db`) |
| `--no-rerun-failed` | Disable rerunning failed tests without Devel::Cover (enabled by default) |
| `-v, --verbose` | Verbose output with uncovered line details |
| `-o <dir>` | Output directory for reports |
| `--source <dir>` | Source directories to measure (default: `lib`) |
| `--ignore <dir>` | Directories to ignore for coverage |
| `--no-select` | Disable `-select` optimization (for benchmarking) |
| `--json-merge` | Force JSON format for coverage data (enables pure Go merging) |
| `--normalize <modes>` | Normalize coverage metrics (see below) |
| `--version` | Show version information |

### Coverage Normalization

The `--normalize` flag transforms coverage metrics to match output formats expected by other tools like SonarQube or JaCoCo. Available modes (can be combined with commas):

| Mode | Description |
|------|-------------|
| `conditions-to-branches` | Merge condition coverage into branch coverage |
| `subroutines-to-statements` | Merge subroutine coverage into statement coverage |
| `sonarqube` | SonarQube-style normalization (conditions→branches, shows combined coverage) |
| `simple` | Show only statement coverage |

```bash
# Merge conditions into branches (like SonarQube)
perlcov --normalize=conditions-to-branches

# Apply full SonarQube-style normalization
perlcov --normalize=sonarqube

# Combine multiple normalizations
perlcov --normalize=conditions-to-branches,subroutines-to-statements
```

## Example Output

```
Using Devel::Cover version 1.51
Found 71 test files

--- Test Results ---
✓ t/accessor-coerce.t (3.54s)
✓ t/accessor-default.t (3.50s)
✓ t/buildargs.t (3.45s)
...

--- Coverage Report ---

File                                          Stmt     Branch       Cond        Sub
------------------------------------------------------------------------------------
lib/Moo.pm                                   97.4%      92.3%      70.9%      75.7%
lib/Moo/Role.pm                              78.4%      72.8%      48.6%      92.8%
lib/Moo/Object.pm                            88.6%     100.0%      46.6%      90.0%
------------------------------------------------------------------------------------
Total                                        80.0%      77.3%      59.1%      85.1%

=== Summary ===
Tests: 71 passed, 0 failed, 71 total
Coverage: 80.0% statement, 77.3% branch
```

## Detecting Devel::Cover-Related Failures

Devel::Cover can sometimes cause tests to fail that would otherwise pass. By default, perlcov automatically reruns failed tests without Devel::Cover to detect these issues:

```
--- Rerun Results (without Devel::Cover) ---
⚠️  t/some-test.t: PASSED without Devel::Cover (coverage-related failure)
✗ t/other-test.t: Still FAILED (genuine test failure)
```

To disable this behavior, use `--no-rerun-failed`.

## How It Works

1. **Test Discovery**: Recursively finds all `.t` files under the specified test directories
2. **Parallel Execution**: Runs tests in parallel using Go goroutines, each with Devel::Cover enabled
3. **Fast Merging**: Coverage databases are read directly in Go (JSON format) and merged without spawning Perl processes
4. **Accurate Reporting**: Coverage percentages match the `cover` command output (verified against Moo test suite)

### JSON Merge Mode

perlcov automatically detects when coverage files are in JSON format and uses pure Go parsing for the merge step. This happens automatically when `JSON::MaybeXS` is installed.

The `--json-merge` flag converts Sereal/Storable coverage databases to JSON format after tests complete, then merges them in pure Go. This is useful when:
- You have `Sereal` installed (which takes priority over JSON by default)
- You want faster merging without installing `JSON::MaybeXS`

### Accuracy

perlcov produces the same coverage numbers as Devel::Cover's `cover` command:

```
cover:   Total  80.0%  77.3%  59.1%  82.2%
perlcov: Total  80.0%  77.4%  59.1%  82.2%
```

Minor rounding differences may occur due to floating-point calculation order.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.
