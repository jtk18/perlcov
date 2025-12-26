# perlcov

A fast, parallel Perl test coverage tool that wraps [Devel::Cover](https://metacpan.org/pod/Devel::Cover).

## Features

- **Parallel test execution** - Run tests concurrently for significant speedups
- **Perly interface** - Familiar `-I` flag for include paths
- **Clean coverage reports** - Readable text output showing statement, branch, condition, and subroutine coverage
- **Failed test detection** - Identify whether test failures are caused by Devel::Cover itself
- **HTML reports** - Optional HTML generation via Devel::Cover's `cover` command

## Performance

Benchmarked against [Moo](https://github.com/moose/Moo) (71 test files, 841 tests):

| Method | Time | Notes |
|--------|------|-------|
| `prove -l t/*.t` (no coverage) | 14s | Baseline |
| `prove -l t/*.t` with Devel::Cover | 173s | 12x slower |
| `perlcov -j 8` | 33s | 5x faster than sequential coverage |

The parallel execution provides significant speedups, especially on machines with multiple cores.

### Note on HTML Reports

Generating HTML reports via the `--html` flag uses the `cover` command, which can be slow for large codebases. For 1000+ test files, HTML generation may take several minutes. The text report is generated instantly using direct database parsing.

## Installation

### Prerequisites

- Go 1.21 or later
- Perl 5.10 or later
- Devel::Cover (`cpan Devel::Cover`)

### Building from Source

```bash
git clone https://github.com/user/perlcov.git
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
| `--version` | Show version information |

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
3. **Coverage Merging**: Devel::Cover automatically merges coverage data from parallel runs
4. **Fast Reporting**: Parses the coverage database directly using Perl's `Devel::Cover::DB` module and outputs clean, readable reports

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.
