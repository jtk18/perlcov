package coverage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Report represents the coverage report
type Report struct {
	Files   map[string]*FileCoverage
	Summary CoverageSummary
}

// FileCoverage represents coverage data for a single file
type FileCoverage struct {
	Path        string
	Statements  StatementCoverage
	Branches    BranchCoverage
	Conditions  ConditionCoverage
	Subroutines SubroutineCoverage
}

// StatementCoverage holds statement coverage data
type StatementCoverage struct {
	Covered   int
	Total     int
	Percent   float64
	Uncovered []int // Line numbers
	// Internal: line -> hit count for merging
	lines map[int]int
}

// BranchCoverage holds branch coverage data
type BranchCoverage struct {
	Covered int
	Total   int
	Percent float64
}

// ConditionCoverage holds condition coverage data
type ConditionCoverage struct {
	Covered int
	Total   int
	Percent float64
}

// SubroutineCoverage holds subroutine coverage data
type SubroutineCoverage struct {
	Covered int
	Total   int
	Percent float64
}

// CoverageSummary holds overall coverage statistics
type CoverageSummary struct {
	Statement    float64
	Branch       float64
	Condition    float64
	Subroutine   float64
	TotalFiles   int
	CoveredFiles int
}

// runCoverageData represents coverage data from a single test run
type runCoverageData struct {
	Files []struct {
		Path      string `json:"path"`
		Statement struct {
			Lines map[string]int `json:"lines"` // line number -> hit count
		} `json:"statement"`
		Branch struct {
			Covered int `json:"covered"`
			Total   int `json:"total"`
		} `json:"branch"`
		Condition struct {
			Covered int `json:"covered"`
			Total   int `json:"total"`
		} `json:"condition"`
		Subroutine struct {
			Covered int `json:"covered"`
			Total   int `json:"total"`
		} `json:"subroutine"`
	} `json:"files"`
}

// ParseCoverageDB parses the Devel::Cover database and returns a report
func ParseCoverageDB(coverDir string) (*Report, error) {
	// Check if cover_db exists
	if _, err := os.Stat(coverDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("coverage directory %s does not exist", coverDir)
	}

	runsDir := filepath.Join(coverDir, "runs")
	if _, err := os.Stat(runsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("no coverage runs found in %s", runsDir)
	}

	// Find all run directories
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read runs directory: %w", err)
	}

	var runDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			runDirs = append(runDirs, filepath.Join(runsDir, entry.Name()))
		}
	}

	if len(runDirs) == 0 {
		return nil, fmt.Errorf("no coverage run directories found")
	}

	// Parse runs in parallel
	type parseResult struct {
		data *runCoverageData
		err  error
	}

	results := make(chan parseResult, len(runDirs))
	var wg sync.WaitGroup

	for _, runDir := range runDirs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			data, err := parseRunDirectory(dir)
			results <- parseResult{data: data, err: err}
		}(runDir)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and merge results
	report := &Report{
		Files: make(map[string]*FileCoverage),
	}

	for result := range results {
		if result.err != nil {
			// Log but continue - some runs might fail
			continue
		}
		if result.data != nil {
			mergeRunData(report, result.data)
		}
	}

	// Calculate final percentages and summary
	calculateSummary(report)

	return report, nil
}

// parseRunDirectory parses a single run directory and returns coverage data
func parseRunDirectory(runDir string) (*runCoverageData, error) {
	// Use Perl to parse the run data and output JSON
	script := `
use strict;
use warnings;
use JSON::PP;

local $SIG{__WARN__} = sub {};

my $run_dir = $ARGV[0];
my %result = (files => []);

# Find the cover data file (Sereal or Storable format)
my $data;
for my $file (glob("$run_dir/cover.*"), glob("$run_dir/*")) {
    next if -d $file;
    eval {
        if (eval { require Sereal::Decoder; 1 }) {
            my $decoder = Sereal::Decoder->new;
            open my $fh, '<:raw', $file or next;
            local $/;
            my $content = <$fh>;
            close $fh;
            $data = $decoder->decode($content);
        }
    };
    last if $data;
    eval {
        require Storable;
        $data = Storable::retrieve($file);
    };
    last if $data;
}

exit 0 unless $data && ref $data eq 'HASH';

# Extract coverage data
my $runs = $data->{runs} || {};
for my $run_id (keys %$runs) {
    my $run = $runs->{$run_id};
    my $cover = $run->{cover} || next;

    for my $file (keys %$cover) {
        my $file_data = $cover->{$file};
        my %file_result = (
            path => $file,
            statement => { lines => {} },
            branch => { covered => 0, total => 0 },
            condition => { covered => 0, total => 0 },
            subroutine => { covered => 0, total => 0 },
        );

        # Statement coverage
        if (my $stmt = $file_data->{statement}) {
            for my $line (keys %$stmt) {
                my $hits = $stmt->{$line};
                $hits = $hits->[0] if ref $hits eq 'ARRAY';
                $file_result{statement}{lines}{$line} = $hits // 0;
            }
        }

        # Branch coverage
        if (my $branch = $file_data->{branch}) {
            for my $line (keys %$branch) {
                my $branches = $branch->{$line};
                next unless ref $branches eq 'ARRAY';
                for my $b (@$branches) {
                    next unless ref $b eq 'ARRAY';
                    $file_result{branch}{total} += 2;
                    $file_result{branch}{covered}++ if $b->[0];
                    $file_result{branch}{covered}++ if $b->[1];
                }
            }
        }

        # Condition coverage
        if (my $cond = $file_data->{condition}) {
            for my $line (keys %$cond) {
                my $conds = $cond->{$line};
                next unless ref $conds eq 'ARRAY';
                for my $c (@$conds) {
                    next unless ref $c eq 'ARRAY';
                    for my $val (@$c) {
                        $file_result{condition}{total}++;
                        $file_result{condition}{covered}++ if $val;
                    }
                }
            }
        }

        # Subroutine coverage
        if (my $sub = $file_data->{subroutine}) {
            for my $line (keys %$sub) {
                my $hits = $sub->{$line};
                $hits = $hits->[0] if ref $hits eq 'ARRAY';
                $file_result{subroutine}{total}++;
                $file_result{subroutine}{covered}++ if $hits && $hits > 0;
            }
        }

        push @{$result{files}}, \%file_result;
    }
}

print JSON::PP->new->utf8->encode(\%result);
`

	cmd := exec.Command("perl", "-e", script, runDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to parse run: %w\nStderr: %s", err, stderr.String())
	}

	if stdout.Len() == 0 {
		return nil, nil // Empty run, skip
	}

	var data runCoverageData
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &data, nil
}

// mergeRunData merges coverage data from a single run into the report
func mergeRunData(report *Report, data *runCoverageData) {
	for _, f := range data.Files {
		fc, exists := report.Files[f.Path]
		if !exists {
			fc = &FileCoverage{
				Path: f.Path,
				Statements: StatementCoverage{
					lines: make(map[int]int),
				},
			}
			report.Files[f.Path] = fc
		}

		// Merge statement coverage (line hits are additive)
		if fc.Statements.lines == nil {
			fc.Statements.lines = make(map[int]int)
		}
		for lineStr, hits := range f.Statement.Lines {
			var line int
			if _, err := fmt.Sscanf(lineStr, "%d", &line); err != nil {
				continue // Skip malformed line numbers
			}
			fc.Statements.lines[line] += hits
		}

		// Merge branch coverage (take max)
		fc.Branches.Total = max(fc.Branches.Total, f.Branch.Total)
		fc.Branches.Covered = max(fc.Branches.Covered, f.Branch.Covered)

		// Merge condition coverage (take max)
		fc.Conditions.Total = max(fc.Conditions.Total, f.Condition.Total)
		fc.Conditions.Covered = max(fc.Conditions.Covered, f.Condition.Covered)

		// Merge subroutine coverage (take max)
		fc.Subroutines.Total = max(fc.Subroutines.Total, f.Subroutine.Total)
		fc.Subroutines.Covered = max(fc.Subroutines.Covered, f.Subroutine.Covered)
	}
}

// calculateSummary calculates final coverage percentages and summary
func calculateSummary(report *Report) {
	var totalStmt, coveredStmt int
	var totalBranch, coveredBranch int
	var totalCond, coveredCond int
	var totalSub, coveredSub int

	for _, fc := range report.Files {
		// Finalize statement coverage from line data
		fc.Statements.Total = len(fc.Statements.lines)
		fc.Statements.Covered = 0
		fc.Statements.Uncovered = nil

		for line, hits := range fc.Statements.lines {
			if hits > 0 {
				fc.Statements.Covered++
			} else {
				fc.Statements.Uncovered = append(fc.Statements.Uncovered, line)
			}
		}
		sort.Ints(fc.Statements.Uncovered)

		// Calculate percentages
		if fc.Statements.Total > 0 {
			fc.Statements.Percent = float64(fc.Statements.Covered) / float64(fc.Statements.Total) * 100
		}
		if fc.Branches.Total > 0 {
			fc.Branches.Percent = float64(fc.Branches.Covered) / float64(fc.Branches.Total) * 100
		}
		if fc.Conditions.Total > 0 {
			fc.Conditions.Percent = float64(fc.Conditions.Covered) / float64(fc.Conditions.Total) * 100
		}
		if fc.Subroutines.Total > 0 {
			fc.Subroutines.Percent = float64(fc.Subroutines.Covered) / float64(fc.Subroutines.Total) * 100
		}

		// Accumulate totals
		totalStmt += fc.Statements.Total
		coveredStmt += fc.Statements.Covered
		totalBranch += fc.Branches.Total
		coveredBranch += fc.Branches.Covered
		totalCond += fc.Conditions.Total
		coveredCond += fc.Conditions.Covered
		totalSub += fc.Subroutines.Total
		coveredSub += fc.Subroutines.Covered

		report.Summary.TotalFiles++
		if fc.Statements.Covered > 0 {
			report.Summary.CoveredFiles++
		}
	}

	// Calculate summary percentages
	if totalStmt > 0 {
		report.Summary.Statement = float64(coveredStmt) / float64(totalStmt) * 100
	}
	if totalBranch > 0 {
		report.Summary.Branch = float64(coveredBranch) / float64(totalBranch) * 100
	}
	if totalCond > 0 {
		report.Summary.Condition = float64(coveredCond) / float64(totalCond) * 100
	}
	if totalSub > 0 {
		report.Summary.Subroutine = float64(coveredSub) / float64(totalSub) * 100
	}
}

// PrintReport prints the coverage report to stdout
func PrintReport(report *Report, verbose bool) {
	// Sort files by path
	var paths []string
	for path := range report.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Print header
	fmt.Printf("\n%-60s %10s %10s %10s %10s\n",
		"File", "Stmt", "Branch", "Cond", "Sub")
	fmt.Println(strings.Repeat("-", 104))

	// Print each file
	for _, path := range paths {
		f := report.Files[path]
		displayPath := path
		if len(displayPath) > 58 {
			displayPath = "..." + displayPath[len(displayPath)-55:]
		}

		stmtStr := formatCoverage(f.Statements.Covered, f.Statements.Total)
		branchStr := formatCoverage(f.Branches.Covered, f.Branches.Total)
		condStr := formatCoverage(f.Conditions.Covered, f.Conditions.Total)
		subStr := formatCoverage(f.Subroutines.Covered, f.Subroutines.Total)

		fmt.Printf("%-60s %10s %10s %10s %10s\n",
			displayPath, stmtStr, branchStr, condStr, subStr)

		// Show uncovered lines in verbose mode
		if verbose && len(f.Statements.Uncovered) > 0 {
			fmt.Printf("    Uncovered lines: %v\n", f.Statements.Uncovered)
		}
	}

	// Print summary
	fmt.Println(strings.Repeat("-", 104))
	fmt.Printf("%-60s %9.1f%% %9.1f%% %9.1f%% %9.1f%%\n",
		"Total",
		report.Summary.Statement,
		report.Summary.Branch,
		report.Summary.Condition,
		report.Summary.Subroutine)
}

func formatCoverage(covered, total int) string {
	if total == 0 {
		return "n/a"
	}
	pct := float64(covered) / float64(total) * 100
	return fmt.Sprintf("%.1f%%", pct)
}

// GenerateHTML generates an HTML report using the cover command
// Note: This is slow because it uses the cover command to merge and render
func GenerateHTML(coverDir, _ string) error {
	fmt.Println("Merging coverage data for HTML report (this may take a while)...")

	// Use the cover command to generate HTML - it will merge runs automatically
	cmd := exec.Command("cover", "-report", "html", coverDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cover command failed: %w", err)
	}

	return nil
}
