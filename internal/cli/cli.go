package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/user/perlcov/internal/coverage"
	"github.com/user/perlcov/internal/runner"
)

// Config holds the CLI configuration
type Config struct {
	IncludePaths  []string
	Jobs          int
	HTML          bool
	CoverDir      string
	NoRerunFailed bool
	Verbose       bool
	TestPaths     []string
	SourceDirs    []string
	OutputDir     string
	ShowVersion   bool
	IgnoreDirs    []string
	NoSelect      bool
	Normalize     string // Comma-separated normalization modes
	JSONMerge     bool   // Use JSON export + Go merging instead of Perl merging
	PerlPath      string // Path to perl executable
}

// Version information
const Version = "0.1.2"

// multiString implements flag.Value for multiple -I flags
type multiString []string

func (m *multiString) String() string {
	return strings.Join(*m, ",")
}

func (m *multiString) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// printFlagDefaults prints flag defaults with -- for long flags
func printFlagDefaults(fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		// Determine prefix: use -- for multi-char flags, - for single-char
		prefix := "-"
		if len(f.Name) > 1 {
			prefix = "--"
		}

		// Format the flag name and default value
		var defaultVal string
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" {
			defaultVal = fmt.Sprintf(" (default %s)", f.DefValue)
		}

		// Print with proper formatting
		fmt.Fprintf(os.Stderr, "  %s%s\n        %s%s\n", prefix, f.Name, f.Usage, defaultVal)
	})
}

// Run executes the CLI with the given arguments
func Run(args []string) error {
	cfg := &Config{}

	fs := flag.NewFlagSet("perlcov", flag.ExitOnError)

	var includePaths multiString
	var ignoreDirs multiString
	var sourceDirs multiString

	fs.Var(&includePaths, "I", "Add directory to @INC (can be specified multiple times)")
	fs.IntVar(&cfg.Jobs, "j", runtime.NumCPU(), "Number of parallel test jobs")
	fs.BoolVar(&cfg.HTML, "html", false, "Generate HTML coverage report (warning: slow)")
	fs.StringVar(&cfg.CoverDir, "cover-dir", "cover_db", "Directory for coverage database")
	fs.BoolVar(&cfg.NoRerunFailed, "no-rerun-failed", false, "Disable rerunning failed tests without Devel::Cover")
	fs.BoolVar(&cfg.Verbose, "v", false, "Verbose output")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")
	fs.StringVar(&cfg.OutputDir, "o", "", "Output directory for reports (default: current directory)")
	fs.BoolVar(&cfg.ShowVersion, "version", false, "Show version information")
	fs.Var(&ignoreDirs, "ignore", "Directories to ignore for coverage (can be specified multiple times)")
	fs.Var(&sourceDirs, "source", "Source directories to measure coverage (default: lib)")
	fs.BoolVar(&cfg.NoSelect, "no-select", false, "Disable -select optimization (for benchmarking)")
	fs.StringVar(&cfg.Normalize, "normalize", "", "Normalize coverage metrics (comma-separated modes: conditions-to-branches, subroutines-to-statements, sonarqube, simple)")
	fs.BoolVar(&cfg.JSONMerge, "json-merge", false, "Export coverage to JSON and merge in Go (faster for large test suites)")
	fs.StringVar(&cfg.PerlPath, "perl-path", "", "Path to perl executable (default: perl from PATH, or $PERL_PATH)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `perlcov - Fast Perl test coverage tool

Usage: perlcov [options] [test-files-or-directories...]

If no test files or directories are specified, perlcov will search for
t/**/*.t (all .t files under the t/ directory, recursively).

Options:
`)
		printFlagDefaults(fs)
		fmt.Fprintf(os.Stderr, `
Examples:
  perlcov                           # Run all tests in t/**/*.t
  perlcov -j 4                      # Run tests with 4 parallel jobs
  perlcov -I lib -I local/lib       # Add include paths
  perlcov --html                    # Generate HTML report (slow)
  perlcov --no-rerun-failed         # Don't rerun failed tests without coverage
  perlcov --no-select               # Disable -select optimization (for benchmarking)
  perlcov --json-merge              # Use JSON export + Go merging (faster)
  perlcov --normalize=conditions-to-branches   # Merge conditions into branches
  perlcov --normalize=sonarqube     # Use SonarQube-style coverage metrics
  perlcov --normalize=simple        # Show only statement coverage
  perlcov --perl-path=/usr/bin/perl # Use specific perl executable
  perlcov t/unit/                   # Run tests in specific directory
  perlcov t/foo.t t/bar.t           # Run specific test files

Environment Variables:
  PERL_PATH                         Path to perl executable (overridden by --perl-path)

Note: This tool requires Devel::Cover to be installed.
      Install with: cpan Devel::Cover

      By default, failed tests are automatically rerun without Devel::Cover
      to detect coverage-related failures. Use --no-rerun-failed to disable.
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.ShowVersion {
		fmt.Printf("perlcov version %s\n", Version)
		return nil
	}

	cfg.IncludePaths = includePaths
	cfg.IgnoreDirs = ignoreDirs
	cfg.SourceDirs = sourceDirs

	// Use PERL_PATH env var as fallback if --perl-path not specified
	if cfg.PerlPath == "" {
		if envPath := os.Getenv("PERL_PATH"); envPath != "" {
			cfg.PerlPath = envPath
		} else {
			cfg.PerlPath = "perl" // default to perl in PATH
		}
	}

	if len(cfg.SourceDirs) == 0 {
		cfg.SourceDirs = []string{"lib"}
	}

	// Remaining args are test paths
	cfg.TestPaths = fs.Args()
	if len(cfg.TestPaths) == 0 {
		cfg.TestPaths = []string{"t"}
	}

	if cfg.OutputDir == "" {
		cfg.OutputDir = "."
	}

	return runCoverage(cfg)
}

func runCoverage(cfg *Config) error {
	// Check for Devel::Cover
	if err := runner.CheckDevelCover(cfg.PerlPath); err != nil {
		return err
	}

	// Discover test files
	testFiles, err := discoverTests(cfg.TestPaths)
	if err != nil {
		return fmt.Errorf("failed to discover tests: %w", err)
	}

	if len(testFiles) == 0 {
		return fmt.Errorf("no test files found")
	}

	fmt.Printf("Found %d test files\n", len(testFiles))

	// Clean previous coverage data (both main dir and any isolated dirs)
	if err := os.RemoveAll(cfg.CoverDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clean coverage directory: %w", err)
	}
	// Also clean any leftover isolated coverage directories from previous runs
	for i := 0; i < len(testFiles); i++ {
		isolatedDir := fmt.Sprintf("%s_%d", cfg.CoverDir, i)
		os.RemoveAll(isolatedDir) // Ignore errors
	}

	// Run tests with coverage (each test gets its own isolated coverage directory)
	r := runner.New(cfg.IncludePaths, cfg.CoverDir, cfg.Jobs, cfg.Verbose, cfg.SourceDirs, cfg.NoSelect, cfg.JSONMerge, cfg.PerlPath)
	results := r.RunTests(testFiles)

	// Collect isolated coverage directories from test results
	var isolatedDirs []string
	for _, result := range results {
		if result.CoverDir != "" {
			isolatedDirs = append(isolatedDirs, result.CoverDir)
		}
	}

	// Merge isolated coverage directories into the final cover_db
	if len(isolatedDirs) > 0 {
		if cfg.Verbose {
			fmt.Printf("Merging %d coverage directories...\n", len(isolatedDirs))
		}
		if err := coverage.MergeCoverageDBs(isolatedDirs, cfg.CoverDir); err != nil {
			return fmt.Errorf("failed to merge coverage directories: %w", err)
		}
	}

	// Print test results
	printTestResults(results)

	// Handle failed tests - rerun by default to detect Devel::Cover-related failures
	failedTests := getFailedTests(results)
	if len(failedTests) > 0 && !cfg.NoRerunFailed {
		fmt.Println("\n--- Rerunning failed tests without Devel::Cover ---")
		rerunResults := r.RunTestsWithoutCoverage(failedTests)
		printRerunResults(results, rerunResults)
	}

	// Parse and display coverage
	fmt.Println("\n--- Coverage Report ---")
	report, err := coverage.ParseCoverageDB(cfg.CoverDir, cfg.JSONMerge, cfg.PerlPath)
	if err != nil {
		return fmt.Errorf("failed to parse coverage: %w", err)
	}

	// Apply normalization if specified
	if cfg.Normalize != "" {
		normConfig, err := coverage.ParseNormalizationModes(cfg.Normalize)
		if err != nil {
			return fmt.Errorf("invalid --normalize value: %w", err)
		}
		report.Normalize(normConfig)
	}

	coverage.PrintReport(report, cfg.Verbose)

	// Generate HTML if requested
	if cfg.HTML {
		fmt.Println("\n⚠️  WARNING: HTML report generation using 'cover' can be very slow")
		fmt.Println("   For large codebases, this may take several minutes...")
		if err := coverage.GenerateHTML(cfg.CoverDir, cfg.OutputDir); err != nil {
			return fmt.Errorf("failed to generate HTML report: %w", err)
		}
		htmlPath := filepath.Join(cfg.OutputDir, cfg.CoverDir, "coverage.html")
		fmt.Printf("\n📊 HTML report generated: %s\n", htmlPath)
	}

	// Summary
	passCount := len(results) - len(failedTests)
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Tests: %d passed, %d failed, %d total\n", passCount, len(failedTests), len(results))
	fmt.Printf("Coverage: %.1f%% statement, %.1f%% branch\n",
		report.Summary.Statement, report.Summary.Branch)

	if len(failedTests) > 0 {
		return fmt.Errorf("%d test(s) failed", len(failedTests))
	}

	return nil
}

func discoverTests(paths []string) ([]string, error) {
	var testFiles []string

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("cannot access %s: %w", p, err)
		}

		if !info.IsDir() {
			// It's a file
			if strings.HasSuffix(p, ".t") {
				testFiles = append(testFiles, p)
			}
			continue
		}

		// It's a directory, find all .t files recursively
		err = filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".t") {
				testFiles = append(testFiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return testFiles, nil
}

func printTestResults(results []runner.TestResult) {
	fmt.Println("\n--- Test Results ---")
	for _, r := range results {
		status := "✓"
		if !r.Passed {
			status = "✗"
		}
		fmt.Printf("%s %s (%.2fs)\n", status, r.File, r.Duration.Seconds())
		if !r.Passed && r.Error != "" {
			// Show first few lines of error
			lines := strings.Split(r.Error, "\n")
			for i, line := range lines {
				if i >= 5 {
					fmt.Printf("      ... (%d more lines)\n", len(lines)-5)
					break
				}
				fmt.Printf("      %s\n", line)
			}
		}
	}
}

func getFailedTests(results []runner.TestResult) []string {
	var failed []string
	for _, r := range results {
		if !r.Passed {
			failed = append(failed, r.File)
		}
	}
	return failed
}

func printRerunResults(original []runner.TestResult, rerun []runner.TestResult) {
	// Create map for quick lookup
	originalResults := make(map[string]bool)
	for _, r := range original {
		originalResults[r.File] = r.Passed
	}

	fmt.Println("\n--- Rerun Results (without Devel::Cover) ---")
	for _, r := range rerun {
		originalPassed := originalResults[r.File]

		if r.Passed && !originalPassed {
			fmt.Printf("⚠️  %s: PASSED without Devel::Cover (coverage-related failure)\n", r.File)
		} else if !r.Passed && !originalPassed {
			fmt.Printf("✗ %s: Still FAILED (genuine test failure)\n", r.File)
		} else {
			fmt.Printf("? %s: Unexpected state\n", r.File)
		}
	}
}
