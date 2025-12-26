package coverage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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

// ParseCoverageDB parses the Devel::Cover database and returns a report
func ParseCoverageDB(coverDir string) (*Report, error) {
	// Check if cover_db exists
	if _, err := os.Stat(coverDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("coverage directory %s does not exist", coverDir)
	}

	// First, merge the coverage runs using cover command
	// This is needed because parallel test execution creates separate run files
	if err := mergeCoverageRuns(coverDir); err != nil {
		return nil, fmt.Errorf("failed to merge coverage runs: %w", err)
	}

	// Use Devel::Cover's JSON output capability
	// First, try to use cover with JSON output
	jsonData, err := extractCoverageJSON(coverDir)
	if err != nil {
		// Fallback to parsing the Sereal database directly
		return parseCoverageSereal(coverDir)
	}

	return parseJSONReport(jsonData)
}

// mergeCoverageRuns merges multiple coverage run files into a single database
func mergeCoverageRuns(coverDir string) error {
	// Use cover command with -silent to merge runs
	// The cover command automatically merges runs when accessing the database
	cmd := exec.Command("cover", "-silent", "-summary", coverDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try a Perl-based merge as fallback
		return mergeCoverageRunsPerl(coverDir)
	}
	_ = output // Ignore output, we just want the merge side effect
	return nil
}

// mergeCoverageRunsPerl merges runs using Perl directly
func mergeCoverageRunsPerl(coverDir string) error {
	script := `
use strict;
use warnings;
use Devel::Cover::DB;

my $db = Devel::Cover::DB->new(db => $ARGV[0]);
# Just loading the DB merges the runs
$db->calculate_summary(map { $_ => 1 } qw(statement branch condition subroutine));
print "Merged\n";
`
	cmd := exec.Command("perl", "-e", script, coverDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("merge failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// extractCoverageJSON tries to extract coverage data as JSON using Perl
func extractCoverageJSON(coverDir string) ([]byte, error) {
	// Use a Perl one-liner to read the coverage database and output JSON
	script := `
use strict;
use warnings;
use JSON::PP;
use Devel::Cover::DB;

# Suppress any warnings to stderr
local $SIG{__WARN__} = sub {};

my $db = Devel::Cover::DB->new(db => $ARGV[0]);
$db->calculate_summary(map { $_ => 1 } qw(statement branch condition subroutine));

my %report;
my $total = $db->summary('Total') || {};
$report{summary} = {
    statement => $total->{statement}{percentage} // 0,
    branch => $total->{branch}{percentage} // 0,
    condition => $total->{condition}{percentage} // 0,
    subroutine => $total->{subroutine}{percentage} // 0,
};

my @files;
for my $file (sort $db->cover->items) {
    my $f = $db->cover->file($file);
    my %file_data = (
        path => $file,
        statement => { covered => 0, total => 0, uncovered => [] },
        branch => { covered => 0, total => 0 },
        condition => { covered => 0, total => 0 },
        subroutine => { covered => 0, total => 0 },
    );

    # Statement coverage
    if (my $stmt = $f->statement) {
        for my $line ($stmt->items) {
            my $count = $stmt->location($line);
            next unless defined $count;
            $file_data{statement}{total}++;
            if ($count->[0] && $count->[0] > 0) {
                $file_data{statement}{covered}++;
            } else {
                push @{$file_data{statement}{uncovered}}, $line;
            }
        }
    }

    # Branch coverage
    if (my $branch = $f->branch) {
        for my $line ($branch->items) {
            my $data = $branch->location($line);
            next unless $data;
            for my $b (@$data) {
                next unless ref $b eq 'ARRAY';
                $file_data{branch}{total} += 2;  # Each branch has true/false
                $file_data{branch}{covered}++ if $b->[0];
                $file_data{branch}{covered}++ if $b->[1];
            }
        }
    }

    # Condition coverage
    if (my $cond = $f->condition) {
        for my $line ($cond->items) {
            my $data = $cond->location($line);
            next unless $data;
            for my $c (@$data) {
                next unless ref $c eq 'ARRAY';
                for my $val (@$c) {
                    $file_data{condition}{total}++;
                    $file_data{condition}{covered}++ if $val;
                }
            }
        }
    }

    # Subroutine coverage
    if (my $sub = $f->subroutine) {
        for my $line ($sub->items) {
            my $count = $sub->location($line);
            next unless defined $count;
            next unless ref $count eq 'ARRAY';
            $file_data{subroutine}{total}++;
            $file_data{subroutine}{covered}++ if $count->[0] && $count->[0] > 0;
        }
    }

    push @files, \%file_data;
}

$report{files} = \@files;
print JSON::PP->new->utf8->encode(\%report);
`

	cmd := exec.Command("perl", "-e", script, coverDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to extract coverage JSON: %w\nStderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// parseCoverageSereal parses the Sereal database format
func parseCoverageSereal(coverDir string) (*Report, error) {
	// Use Sereal::Decoder to convert to JSON
	script := `
use strict;
use warnings;
use JSON::PP;

# Try to load Sereal, fall back to Storable
my $data;
my $db_file = "$ARGV[0]/cover.14";  # Devel::Cover DB format version

unless (-f $db_file) {
    # Try to find the correct DB file
    opendir(my $dh, $ARGV[0]) or die "Cannot open $ARGV[0]: $!";
    my @files = grep { /^cover\.\d+$/ } readdir($dh);
    closedir($dh);
    if (@files) {
        $db_file = "$ARGV[0]/$files[0]";
    } else {
        die "No cover database found in $ARGV[0]";
    }
}

eval {
    require Sereal::Decoder;
    my $decoder = Sereal::Decoder->new;
    open my $fh, '<:raw', $db_file or die "Cannot open $db_file: $!";
    local $/;
    my $content = <$fh>;
    close $fh;
    $data = $decoder->decode($content);
};

if ($@) {
    # Try Storable
    require Storable;
    $data = Storable::retrieve($db_file);
}

die "Could not load coverage data" unless $data;
print JSON::PP->new->utf8->encode($data);
`

	cmd := exec.Command("perl", "-e", script, coverDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to parse Sereal database: %w\nOutput: %s", err, string(output))
	}

	// Parse the raw structure
	return parseRawCoverageData(output)
}

// parseJSONReport parses the JSON coverage report
func parseJSONReport(data []byte) (*Report, error) {
	var raw struct {
		Summary struct {
			Statement  float64 `json:"statement"`
			Branch     float64 `json:"branch"`
			Condition  float64 `json:"condition"`
			Subroutine float64 `json:"subroutine"`
		} `json:"summary"`
		Files []struct {
			Path      string `json:"path"`
			Statement struct {
				Covered   int   `json:"covered"`
				Total     int   `json:"total"`
				Uncovered []int `json:"uncovered"`
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

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	report := &Report{
		Files: make(map[string]*FileCoverage),
		Summary: CoverageSummary{
			Statement:  raw.Summary.Statement,
			Branch:     raw.Summary.Branch,
			Condition:  raw.Summary.Condition,
			Subroutine: raw.Summary.Subroutine,
		},
	}

	for _, f := range raw.Files {
		fc := &FileCoverage{
			Path: f.Path,
			Statements: StatementCoverage{
				Covered:   f.Statement.Covered,
				Total:     f.Statement.Total,
				Uncovered: f.Statement.Uncovered,
			},
			Branches: BranchCoverage{
				Covered: f.Branch.Covered,
				Total:   f.Branch.Total,
			},
			Conditions: ConditionCoverage{
				Covered: f.Condition.Covered,
				Total:   f.Condition.Total,
			},
			Subroutines: SubroutineCoverage{
				Covered: f.Subroutine.Covered,
				Total:   f.Subroutine.Total,
			},
		}

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

		report.Files[f.Path] = fc
		report.Summary.TotalFiles++
		if fc.Statements.Percent > 0 {
			report.Summary.CoveredFiles++
		}
	}

	return report, nil
}

// parseRawCoverageData parses raw Devel::Cover data structure
func parseRawCoverageData(data []byte) (*Report, error) {
	// This is a simplified parser for the raw structure
	// The actual structure is complex, so we'll return a basic report
	report := &Report{
		Files: make(map[string]*FileCoverage),
	}

	// For now, return an empty report with a note
	// The proper implementation would parse the complex Perl data structure
	return report, nil
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
func GenerateHTML(coverDir, _ string) error {
	// Use the cover command to generate HTML
	cmd := exec.Command("cover", "-report", "html", coverDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cover command failed: %w", err)
	}

	return nil
}
