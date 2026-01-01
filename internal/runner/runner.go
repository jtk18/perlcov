package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TestResult holds the result of running a single test
type TestResult struct {
	File      string
	Passed    bool
	Error     string
	Output    string
	Duration  time.Duration
	CoverDir  string // The isolated coverage directory used for this test
}

// Runner runs Perl tests with optional coverage
type Runner struct {
	IncludePaths []string
	CoverDir     string
	Jobs         int
	Verbose      bool
	SourceDirs   []string
	NoSelect     bool
	JSONMerge    bool   // Use JSON format for coverage data (enables pure Go merging)
	PerlPath     string // Path to perl executable
	ShowOutput   bool   // Show test output during execution
}

// New creates a new Runner
func New(includePaths []string, coverDir string, jobs int, verbose bool, sourceDirs []string, noSelect bool, jsonMerge bool, perlPath string, showOutput bool) *Runner {
	return &Runner{
		IncludePaths: includePaths,
		CoverDir:     coverDir,
		Jobs:         jobs,
		Verbose:      verbose,
		SourceDirs:   sourceDirs,
		NoSelect:     noSelect,
		JSONMerge:    jsonMerge,
		PerlPath:     perlPath,
		ShowOutput:   showOutput,
	}
}

// CheckDevelCover verifies that Devel::Cover is installed
func CheckDevelCover(perlPath string) error {
	// Use -silent,1 to suppress verbose output and -ignore with pattern to ignore -e files
	// The pattern ^\\-e$ matches the literal string "-e" that Devel::Cover sees
	cmd := exec.Command(perlPath, "-MDevel::Cover=-silent,1,-ignore,^\\-e$", "-e", "print $Devel::Cover::VERSION")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Devel::Cover is not installed. Install with: cpan Devel::Cover\nError: %s", string(output))
	}
	fmt.Printf("Using Devel::Cover version %s\n", strings.TrimSpace(string(output)))
	return nil
}

// RunTests runs all test files with coverage
// Each test file gets its own isolated coverage directory to avoid conflicts
// when multiple tests exercise the same source files
func (r *Runner) RunTests(testFiles []string) []TestResult {
	results := make([]TestResult, len(testFiles))
	total := len(testFiles)

	// Create a channel for jobs
	jobs := make(chan int, len(testFiles))
	for i := range testFiles {
		jobs <- i
	}
	close(jobs)

	// Track progress
	var completed int
	var passed int

	// Run tests in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	for w := 0; w < r.Jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// Each test gets an isolated coverage directory
				isolatedCoverDir := fmt.Sprintf("%s_%d", r.CoverDir, i)
				result := r.runSingleTest(testFiles[i], true, isolatedCoverDir)
				mu.Lock()
				results[i] = result
				completed++
				current := completed
				if result.Passed {
					passed++
				}
				currentPassed := passed
				// Print progress every 10 tests or for the last one
				if current%10 == 0 || int(current) == total {
					fmt.Printf("\rProgress: %d/%d tests completed (%d passed, %d failed)   ",
						current, total, currentPassed, current-currentPassed)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	fmt.Println() // Newline after progress
	return results
}

// RunTestsWithoutCoverage runs tests without Devel::Cover
func (r *Runner) RunTestsWithoutCoverage(testFiles []string) []TestResult {
	results := make([]TestResult, len(testFiles))
	total := len(testFiles)

	jobs := make(chan int, len(testFiles))
	for i := range testFiles {
		jobs <- i
	}
	close(jobs)

	// Track progress
	var completed int
	var passed int

	var wg sync.WaitGroup
	var mu sync.Mutex

	for w := 0; w < r.Jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// No coverage directory needed when running without coverage
				result := r.runSingleTest(testFiles[i], false, "")
				mu.Lock()
				results[i] = result
				completed++
				current := completed
				if result.Passed {
					passed++
				}
				currentPassed := passed
				// Print progress every 10 tests or for the last one
				if current%10 == 0 || int(current) == total {
					fmt.Printf("\rProgress: %d/%d tests completed (%d passed, %d failed)   ",
						current, total, currentPassed, current-currentPassed)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	fmt.Println() // Newline after progress
	return results
}

func (r *Runner) runSingleTest(testFile string, withCoverage bool, coverDir string) TestResult {
	start := time.Now()

	// Get absolute paths for everything
	cwd, _ := os.Getwd()
	absCoverDir := coverDir
	if absCoverDir == "" {
		absCoverDir = r.CoverDir
	}
	if !filepath.IsAbs(absCoverDir) {
		absCoverDir = filepath.Join(cwd, absCoverDir)
	}

	absTestFile := testFile
	if !filepath.IsAbs(absTestFile) {
		absTestFile = filepath.Join(cwd, absTestFile)
	}

	args := []string{}

	// Add include paths (convert to absolute)
	for _, inc := range r.IncludePaths {
		absInc := inc
		if !filepath.IsAbs(absInc) {
			absInc = filepath.Join(cwd, absInc)
		}
		args = append(args, "-I", absInc)
	}

	// Always add lib to include path if it exists
	libPath := filepath.Join(cwd, "lib")
	if _, err := os.Stat(libPath); err == nil {
		args = append(args, "-I", libPath)
	}

	if withCoverage {
		// Build Devel::Cover options with absolute path
		coverOpts := fmt.Sprintf("-db,%s,-silent,1,-ignore,^t/,-ignore,\\.t$", absCoverDir)

		// Add source directories to coverage (as absolute paths)
		for _, src := range r.SourceDirs {
			absSrc := src
			if !filepath.IsAbs(absSrc) {
				absSrc = filepath.Join(cwd, absSrc)
			}
			coverOpts += fmt.Sprintf(",+inc,%s", absSrc)
		}

		// Try to derive module name from test filename for targeted coverage
		// Skip this optimization if NoSelect is enabled (for benchmarking)
		if !r.NoSelect {
			if moduleName := extractModuleFromTestFile(testFile); moduleName != "" {
				// Convert Module::Name to Module/Name.pm for file path matching
				moduleFile := strings.ReplaceAll(moduleName, "::", "/") + ".pm"
				// Check if module exists in lib or source directories
				if moduleExists(moduleFile, cwd, r.SourceDirs) {
					// Use -ignore to exclude lib/ files, then -select to include just
					// the target module. The order matters: -ignore must come before
					// -select for Devel::Cover to properly filter.
					modulePattern := strings.TrimSuffix(moduleFile, ".pm")
					coverOpts += fmt.Sprintf(",-ignore,lib/,-select,%s", modulePattern)
					if r.Verbose {
						fmt.Printf("  [select] %s -> %s\n", testFile, moduleName)
					}
				}
			}
		}

		args = append(args, "-MDevel::Cover="+coverOpts)
	}

	args = append(args, absTestFile)

	cmd := exec.Command(r.PerlPath, args...)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	if r.ShowOutput {
		// Stream output to terminal while also capturing it
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	duration := time.Since(start)

	result := TestResult{
		File:     testFile,
		Duration: duration,
		Output:   stdout.String(),
	}

	// Record the coverage directory used for this test
	if withCoverage {
		result.CoverDir = absCoverDir
	}

	if err != nil {
		result.Passed = false
		result.Error = stderr.String()
		if result.Error == "" {
			result.Error = stdout.String()
		}
	} else {
		// Check for TAP failures even if exit code is 0
		result.Passed = !containsTAPFailure(stdout.String())
		if !result.Passed {
			result.Error = stdout.String()
		}
	}

	return result
}

// extractModuleFromTestFile attempts to derive a module name from a test filename
// Pattern: Module-Install-Something.t -> Module::Install::Something
// Pattern: Module-Install-Something_specifier.t -> Module::Install::Something
// Pattern: Module.t -> Module
// Pattern: Module_specifier.t -> Module
// Returns empty string if the pattern doesn't match
func extractModuleFromTestFile(testFile string) string {
	// Get the base filename without directory
	base := filepath.Base(testFile)

	// Strip .t extension
	if !strings.HasSuffix(base, ".t") {
		return ""
	}
	name := strings.TrimSuffix(base, ".t")

	// Skip numbered test files (e.g., 00-load.t, 01-basic.t)
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		return ""
	}

	// Strip anything from the first underscore onwards (specifier portion)
	if idx := strings.Index(name, "_"); idx != -1 {
		name = name[:idx]
	}

	// First character must be uppercase (Perl module naming convention)
	if len(name) == 0 || name[0] < 'A' || name[0] > 'Z' {
		return ""
	}

	// Replace hyphens with :: to form module name
	moduleName := strings.ReplaceAll(name, "-", "::")

	return moduleName
}

// moduleExists checks if a module file exists in cwd, lib, or any of the source directories
func moduleExists(moduleFile, cwd string, sourceDirs []string) bool {
	// Check in cwd first
	cwdPath := filepath.Join(cwd, moduleFile)
	if _, err := os.Stat(cwdPath); err == nil {
		return true
	}

	// Check in lib directory
	libPath := filepath.Join(cwd, "lib", moduleFile)
	if _, err := os.Stat(libPath); err == nil {
		return true
	}

	// Check in source directories
	for _, src := range sourceDirs {
		var srcPath string
		if filepath.IsAbs(src) {
			srcPath = filepath.Join(src, moduleFile)
		} else {
			srcPath = filepath.Join(cwd, src, moduleFile)
		}
		if _, err := os.Stat(srcPath); err == nil {
			return true
		}
	}

	return false
}

// containsTAPFailure checks if the output contains TAP failure indicators
func containsTAPFailure(output string) bool {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Check for "not ok" without "# TODO" or "# SKIP"
		if strings.HasPrefix(line, "not ok") {
			if !strings.Contains(line, "# TODO") && !strings.Contains(line, "# SKIP") {
				return true
			}
		}
		// Check for Bail out
		if strings.HasPrefix(line, "Bail out!") {
			return true
		}
	}
	return false
}
