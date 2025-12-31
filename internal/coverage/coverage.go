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
)

// NormalizationMode represents a coverage normalization transformation
type NormalizationMode string

const (
	// NormalizeConditionsToBranches merges condition coverage into branch coverage
	// This is similar to SonarQube's approach where conditions are reported as branches
	NormalizeConditionsToBranches NormalizationMode = "conditions-to-branches"

	// NormalizeSubroutinesToStatements merges subroutine coverage into statement coverage
	NormalizeSubroutinesToStatements NormalizationMode = "subroutines-to-statements"

	// NormalizeSonarQube applies SonarQube-style normalization:
	// - Conditions merged into branches
	// - Combined coverage = (CT + CF + LC) / (2*B + EL)
	NormalizeSonarQube NormalizationMode = "sonarqube"

	// NormalizeSimple collapses everything to just statement coverage
	NormalizeSimple NormalizationMode = "simple"
)

// NormalizationConfig holds the active normalization modes
type NormalizationConfig struct {
	Modes              []NormalizationMode
	ConditionsToBranch bool // conditions absorbed into branches
	SubroutinesToStmt  bool // subroutines absorbed into statements
	SonarQubeStyle     bool // use SonarQube combined formula
	SimpleMode         bool // only show statement coverage
}

// ParseNormalizationModes parses a comma-separated list of normalization modes
func ParseNormalizationModes(input string) (*NormalizationConfig, error) {
	if input == "" {
		return &NormalizationConfig{}, nil
	}

	config := &NormalizationConfig{}
	modes := strings.Split(input, ",")

	for _, mode := range modes {
		mode = strings.TrimSpace(mode)
		switch NormalizationMode(mode) {
		case NormalizeConditionsToBranches:
			config.ConditionsToBranch = true
			config.Modes = append(config.Modes, NormalizeConditionsToBranches)
		case NormalizeSubroutinesToStatements:
			config.SubroutinesToStmt = true
			config.Modes = append(config.Modes, NormalizeSubroutinesToStatements)
		case NormalizeSonarQube:
			config.SonarQubeStyle = true
			config.ConditionsToBranch = true // SonarQube also merges conditions
			config.Modes = append(config.Modes, NormalizeSonarQube)
		case NormalizeSimple:
			config.SimpleMode = true
			config.Modes = append(config.Modes, NormalizeSimple)
		default:
			return nil, fmt.Errorf("unknown normalization mode: %s (valid: conditions-to-branches, subroutines-to-statements, sonarqube, simple)", mode)
		}
	}

	return config, nil
}

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
	Combined     float64 // SonarQube-style combined coverage
	TotalFiles   int
	CoveredFiles int

	// Normalization state
	Normalized          bool
	ConditionsAbsorbed  bool // conditions merged into branches
	SubroutinesAbsorbed bool // subroutines merged into statements
}

// runCoverageData represents coverage data from a single test run
type runCoverageData struct {
	Files []struct {
		Path      string `json:"path"`
		Statement struct {
			Lines   map[string]int `json:"lines"`   // line number -> hit count (for uncovered lines display)
			Covered int            `json:"covered"` // total covered statements
			Total   int            `json:"total"`   // total statements
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
// If jsonMerge is true, uses pure Go to read JSON files and merge
func ParseCoverageDB(coverDir string, jsonMerge bool, perlPath string) (*Report, error) {
	// Check if cover_db exists
	if _, err := os.Stat(coverDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("coverage directory %s does not exist", coverDir)
	}

	runsDir := filepath.Join(coverDir, "runs")
	if _, err := os.Stat(runsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("no coverage runs found in %s", runsDir)
	}

	// Detect file format by checking first run's cover file
	isJSON := detectJSONFormat(runsDir)

	// If jsonMerge is requested and files aren't JSON, convert them first
	if jsonMerge && !isJSON {
		if err := convertToJSON(coverDir, perlPath); err != nil {
			return nil, fmt.Errorf("failed to convert to JSON: %w", err)
		}
		isJSON = true // Now they're JSON
	}

	var data *runCoverageData
	var err error

	if isJSON {
		// Use pure Go to read JSON files and merge
		data, err = parseAllRunsJSON(coverDir)
	} else {
		// Use Perl to merge Storable/Sereal files
		data, err = parseAllRuns(coverDir, perlPath)
	}
	if err != nil {
		return nil, err
	}

	// Build report from merged data
	report := &Report{
		Files: make(map[string]*FileCoverage),
	}

	for _, f := range data.Files {
		fc := &FileCoverage{
			Path: f.Path,
			Statements: StatementCoverage{
				Covered: f.Statement.Covered,
				Total:   f.Statement.Total,
				lines:   make(map[int]int),
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

		// Build uncovered lines map
		for lineStr := range f.Statement.Lines {
			var line int
			if _, err := fmt.Sscanf(lineStr, "%d", &line); err != nil {
				continue
			}
			fc.Statements.lines[line] = 0
		}

		report.Files[f.Path] = fc
	}

	// Calculate final percentages and summary
	calculateSummary(report)

	return report, nil
}

// convertToJSON converts coverage files from Sereal/Storable to JSON format
// This uses Devel::Cover's IO module to read and JSON::PP (core) to write
func convertToJSON(coverDir string, perlPath string) error {
	script := `
use strict;
use warnings;
use JSON::PP;

my $json = JSON::PP->new->utf8->canonical;
my $cover_db = $ARGV[0];

sub convert_file {
    my ($file) = @_;

    # Skip if already JSON
    open my $fh, '<:raw', $file or return;
    my $first = '';
    read($fh, $first, 1);
    close $fh;
    return if $first eq '{';

    # Read with Devel::Cover::DB::IO (auto-detects format)
    my $data;
    eval {
        require Devel::Cover::DB::IO;
        my $io = Devel::Cover::DB::IO->new;
        $data = $io->read($file);
    };
    return unless $data && ref $data;

    # Write as JSON using JSON::PP
    open my $out, '>:raw', $file or die "Cannot write $file: $!";
    print $out $json->encode($data);
    close $out;
    warn "Converted $file to JSON\n" if $ENV{PERLCOV_VERBOSE};
}

# Convert run files
for my $run_dir (glob("$cover_db/runs/*")) {
    next unless -d $run_dir;
    for my $file (glob("$run_dir/cover.*"), glob("$run_dir/*")) {
        next if -d $file || $file =~ /\.lock$/;
        convert_file($file);
    }
}

# Convert structure files
for my $file (glob("$cover_db/structure/*")) {
    next if -d $file || $file =~ /\.lock$/;
    convert_file($file);
}

print "ok\n";
`

	cmd := exec.Command(perlPath, "-e", script, coverDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to convert coverage to JSON: %w\nStderr: %s", err, stderr.String())
	}

	return nil
}

// detectJSONFormat checks if the coverage files are in JSON format
func detectJSONFormat(runsDir string) bool {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(runsDir, entry.Name())
		files, err := os.ReadDir(runDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || strings.HasSuffix(f.Name(), ".lock") {
				continue
			}
			if !strings.HasPrefix(f.Name(), "cover.") {
				continue
			}

			// Read first few bytes to detect format
			coverPath := filepath.Join(runDir, f.Name())
			file, err := os.Open(coverPath)
			if err != nil {
				continue
			}
			defer file.Close()

			buf := make([]byte, 1)
			n, err := file.Read(buf)
			if err != nil || n == 0 {
				continue
			}

			// JSON files start with '{'
			return buf[0] == '{'
		}
	}

	return false
}

// parseAllRuns parses all run directories and merges coverage data
func parseAllRuns(coverDir string, perlPath string) (*runCoverageData, error) {
	// Use Perl to parse all runs and merge - this is more accurate than merging in Go
	script := `
use strict;
use warnings;
use JSON::PP;

local $SIG{__WARN__} = sub {};

my $cover_db = $ARGV[0];
my %merged;  # file -> { stmt => [], branch => [], cond => [], sub => [] }

# Load structure files to map indices to line numbers
my %structures;
for my $struct_file (glob("$cover_db/structure/*")) {
    next if -d $struct_file || $struct_file =~ /\.lock$/;
    my $struct;
    eval { require Storable; $struct = Storable::retrieve($struct_file); };
    next unless $struct && ref $struct eq 'HASH' && $struct->{file};
    $structures{$struct->{file}} = $struct;
}

# Process all run directories
for my $run_dir (glob("$cover_db/runs/*")) {
    next unless -d $run_dir;

    # Find and load the cover data file
    my $data;
    for my $file (glob("$run_dir/cover.*"), glob("$run_dir/*")) {
        next if -d $file || $file =~ /\.lock$/;
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
    next unless $data && ref $data eq 'HASH';

    # Merge coverage data from this run
    my $runs = $data->{runs} || {};
    for my $run_id (keys %$runs) {
        my $run = $runs->{$run_id};
        my $count = $run->{count} || next;

        for my $file (keys %$count) {
            my $file_count = $count->{$file};

            # Initialize merged data for this file if needed
            if (!$merged{$file}) {
                $merged{$file} = {
                    stmt => [],
                    branch => [],
                    cond => [],
                    sub => [],
                };
            }

            # Merge statement counts (add hits)
            if (my $stmt = $file_count->{statement}) {
                for my $i (0 .. $#$stmt) {
                    $merged{$file}{stmt}[$i] = ($merged{$file}{stmt}[$i] // 0) + ($stmt->[$i] // 0);
                }
            }

            # Merge branch counts (add hits per direction)
            if (my $branch = $file_count->{branch}) {
                for my $i (0 .. $#$branch) {
                    next unless ref $branch->[$i] eq 'ARRAY';
                    $merged{$file}{branch}[$i] //= [0, 0];
                    $merged{$file}{branch}[$i][0] += $branch->[$i][0] // 0;
                    $merged{$file}{branch}[$i][1] += $branch->[$i][1] // 0;
                }
            }

            # Merge condition counts (add hits per state)
            if (my $cond = $file_count->{condition}) {
                for my $i (0 .. $#$cond) {
                    next unless ref $cond->[$i] eq 'ARRAY';
                    $merged{$file}{cond}[$i] //= [];
                    for my $j (0 .. $#{$cond->[$i]}) {
                        $merged{$file}{cond}[$i][$j] = ($merged{$file}{cond}[$i][$j] // 0) + ($cond->[$i][$j] // 0);
                    }
                }
            }

            # Merge subroutine counts (add hits)
            if (my $sub = $file_count->{subroutine}) {
                for my $i (0 .. $#$sub) {
                    $merged{$file}{sub}[$i] = ($merged{$file}{sub}[$i] // 0) + ($sub->[$i] // 0);
                }
            }
        }
    }
}

# Convert merged data to output format
my @files;
for my $file (sort keys %merged) {
    my $m = $merged{$file};
    my $struct = $structures{$file};

    my %file_result = (
        path => $file,
        statement => { lines => {}, covered => 0, total => 0 },
        branch => { covered => 0, total => 0 },
        condition => { covered => 0, total => 0 },
        subroutine => { covered => 0, total => 0 },
    );

    # Count statement coverage
    my $stmt_lines = $struct && $struct->{statement} ? $struct->{statement} : [];
    $file_result{statement}{total} = scalar(@{$m->{stmt}});
    for my $i (0 .. $#{$m->{stmt}}) {
        my $line = $stmt_lines->[$i] // ($i + 1);
        if ($m->{stmt}[$i] && $m->{stmt}[$i] > 0) {
            $file_result{statement}{covered}++;
        } else {
            $file_result{statement}{lines}{$line} = 0;
        }
    }

    # Count branch coverage
    for my $branch (@{$m->{branch}}) {
        next unless ref $branch eq 'ARRAY';
        $file_result{branch}{total} += 2;
        $file_result{branch}{covered}++ if $branch->[0] && $branch->[0] > 0;
        $file_result{branch}{covered}++ if $branch->[1] && $branch->[1] > 0;
    }

    # Count condition coverage
    for my $cond (@{$m->{cond}}) {
        next unless ref $cond eq 'ARRAY';
        for my $val (@$cond) {
            $file_result{condition}{total}++;
            $file_result{condition}{covered}++ if $val && $val > 0;
        }
    }

    # Count subroutine coverage
    for my $hits (@{$m->{sub}}) {
        $file_result{subroutine}{total}++;
        $file_result{subroutine}{covered}++ if $hits && $hits > 0;
    }

    push @files, \%file_result;
}

print JSON::PP->new->utf8->encode({ files => \@files });
`

	cmd := exec.Command(perlPath, "-e", script, coverDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to parse coverage: %w\nStderr: %s", err, stderr.String())
	}

	if stdout.Len() == 0 {
		return &runCoverageData{}, nil
	}

	var data runCoverageData
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &data, nil
}

// singleRunData represents coverage data from a single run (JSON format)
type singleRunData struct {
	File      string         `json:"file"`
	Statement []int          `json:"statement"` // hit counts per line index
	Branch    [][2]int       `json:"branch"`    // [true_hits, false_hits] per branch
	Condition [][]int        `json:"condition"` // hits per condition state
	Sub       []int          `json:"subroutine"`
}

// jsonRunFile represents the JSON format Devel::Cover writes when DEVEL_COVER_DB_FORMAT=JSON
type jsonRunFile struct {
	Runs map[string]struct {
		Count map[string]struct {
			Statement  []int       `json:"statement"`
			Branch     [][]float64 `json:"branch"`    // float64 because Devel::Cover may output e.g. 25.0
			Condition  [][]float64 `json:"condition"` // float64 for consistency
			Subroutine []int       `json:"subroutine"`
		} `json:"count"`
	} `json:"runs"`
}

// jsonStructureFile represents the structure JSON format
type jsonStructureFile struct {
	File      string `json:"file"`
	Statement []int  `json:"statement"`
}

// parseAllRunsJSON reads JSON coverage files directly (no Perl required)
// This works when DEVEL_COVER_DB_FORMAT=JSON is set during test runs
func parseAllRunsJSON(coverDir string) (*runCoverageData, error) {
	runsDir := filepath.Join(coverDir, "runs")
	structDir := filepath.Join(coverDir, "structure")

	// Load structure files for line number mapping
	structures := make(map[string][]int)
	structEntries, err := os.ReadDir(structDir)
	if err == nil {
		for _, entry := range structEntries {
			if entry.IsDir() || strings.HasSuffix(entry.Name(), ".lock") {
				continue
			}
			structPath := filepath.Join(structDir, entry.Name())
			data, err := os.ReadFile(structPath)
			if err != nil {
				continue
			}
			var structFile jsonStructureFile
			if err := json.Unmarshal(data, &structFile); err != nil {
				continue
			}
			if structFile.File != "" {
				structures[structFile.File] = structFile.Statement
			}
		}
	}

	// Find and read all run directories
	runEntries, err := os.ReadDir(runsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read runs directory: %w", err)
	}

	var allRuns [][]singleRunData

	for _, entry := range runEntries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(runsDir, entry.Name())

		// Find the cover.* file in this run directory
		files, err := os.ReadDir(runDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || strings.HasSuffix(f.Name(), ".lock") {
				continue
			}
			if !strings.HasPrefix(f.Name(), "cover.") {
				continue
			}

			coverPath := filepath.Join(runDir, f.Name())
			data, err := os.ReadFile(coverPath)
			if err != nil {
				continue
			}

			var runFile jsonRunFile
			if err := json.Unmarshal(data, &runFile); err != nil {
				continue
			}

			// Extract coverage data from all runs in this file
			for _, run := range runFile.Runs {
				var runData []singleRunData
				for file, counts := range run.Count {
					rd := singleRunData{
						File:      file,
						Statement: counts.Statement,
						Sub:       counts.Subroutine,
					}

					// Convert branch format (float64 -> int)
					for _, b := range counts.Branch {
						if len(b) >= 2 {
							rd.Branch = append(rd.Branch, [2]int{int(b[0]), int(b[1])})
						} else {
							rd.Branch = append(rd.Branch, [2]int{0, 0})
						}
					}

					// Convert condition format (float64 -> int)
					for _, c := range counts.Condition {
						cond := make([]int, len(c))
						for i, v := range c {
							cond[i] = int(v)
						}
						rd.Condition = append(rd.Condition, cond)
					}

					runData = append(runData, rd)
				}
				if len(runData) > 0 {
					allRuns = append(allRuns, runData)
				}
			}
			break // Only need one cover file per run
		}
	}

	// Merge all runs in Go
	return mergeRunsGo(allRuns, structures)
}

// mergeRunsGo merges coverage data from multiple runs in Go
func mergeRunsGo(allRuns [][]singleRunData, structures map[string][]int) (*runCoverageData, error) {
	// Merged data per file
	type mergedFile struct {
		stmt   []int
		branch [][2]int
		cond   [][]int
		sub    []int
	}

	merged := make(map[string]*mergedFile)

	// Merge all runs
	for _, runs := range allRuns {
		for _, r := range runs {
			m, exists := merged[r.File]
			if !exists {
				m = &mergedFile{
					stmt:   make([]int, len(r.Statement)),
					branch: make([][2]int, len(r.Branch)),
					cond:   make([][]int, len(r.Condition)),
					sub:    make([]int, len(r.Sub)),
				}
				// Initialize condition slices
				for i, c := range r.Condition {
					m.cond[i] = make([]int, len(c))
				}
				merged[r.File] = m
			}

			// Extend slices if needed
			for len(m.stmt) < len(r.Statement) {
				m.stmt = append(m.stmt, 0)
			}
			for len(m.branch) < len(r.Branch) {
				m.branch = append(m.branch, [2]int{0, 0})
			}
			for len(m.sub) < len(r.Sub) {
				m.sub = append(m.sub, 0)
			}
			for len(m.cond) < len(r.Condition) {
				m.cond = append(m.cond, nil)
			}

			// Add statement counts
			for i, v := range r.Statement {
				m.stmt[i] += v
			}

			// Add branch counts
			for i, b := range r.Branch {
				m.branch[i][0] += b[0]
				m.branch[i][1] += b[1]
			}

			// Add condition counts
			for i, c := range r.Condition {
				if m.cond[i] == nil {
					m.cond[i] = make([]int, len(c))
				}
				for len(m.cond[i]) < len(c) {
					m.cond[i] = append(m.cond[i], 0)
				}
				for j, v := range c {
					m.cond[i][j] += v
				}
			}

			// Add subroutine counts
			for i, v := range r.Sub {
				m.sub[i] += v
			}
		}
	}

	// Convert to output format
	var files []struct {
		Path      string `json:"path"`
		Statement struct {
			Lines   map[string]int `json:"lines"`
			Covered int            `json:"covered"`
			Total   int            `json:"total"`
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
	}

	for file, m := range merged {
		f := struct {
			Path      string `json:"path"`
			Statement struct {
				Lines   map[string]int `json:"lines"`
				Covered int            `json:"covered"`
				Total   int            `json:"total"`
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
		}{
			Path: file,
		}
		f.Statement.Lines = make(map[string]int)

		// Get line mappings from structure
		stmtLines := structures[file]

		// Count statement coverage
		f.Statement.Total = len(m.stmt)
		for i, hits := range m.stmt {
			line := i + 1 // Default: 1-indexed
			if i < len(stmtLines) {
				line = stmtLines[i]
			}
			if hits > 0 {
				f.Statement.Covered++
			} else {
				f.Statement.Lines[fmt.Sprintf("%d", line)] = 0
			}
		}

		// Count branch coverage
		for _, b := range m.branch {
			f.Branch.Total += 2
			if b[0] > 0 {
				f.Branch.Covered++
			}
			if b[1] > 0 {
				f.Branch.Covered++
			}
		}

		// Count condition coverage
		for _, c := range m.cond {
			for _, hits := range c {
				f.Condition.Total++
				if hits > 0 {
					f.Condition.Covered++
				}
			}
		}

		// Count subroutine coverage
		for _, hits := range m.sub {
			f.Subroutine.Total++
			if hits > 0 {
				f.Subroutine.Covered++
			}
		}

		files = append(files, f)
	}

	// Sort files by path for consistent output
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return &runCoverageData{Files: files}, nil
}

// calculateSummary calculates final coverage percentages and summary
func calculateSummary(report *Report) {
	var totalStmt, coveredStmt int
	var totalBranch, coveredBranch int
	var totalCond, coveredCond int
	var totalSub, coveredSub int

	for _, fc := range report.Files {
		// Build uncovered lines list from the lines map (for verbose display)
		fc.Statements.Uncovered = nil
		for line := range fc.Statements.lines {
			fc.Statements.Uncovered = append(fc.Statements.Uncovered, line)
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

	// Calculate SonarQube-style combined coverage:
	// Coverage = (CT + CF + LC) / (2*B + EL)
	// Where: CT = conditions true, CF = conditions false (we approximate with covered conditions)
	//        LC = lines covered, B = branches, EL = executable lines
	// Simplified: (coveredCond + coveredStmt) / (totalCond + totalStmt)
	combinedTotal := totalCond + totalStmt
	combinedCovered := coveredCond + coveredStmt
	if combinedTotal > 0 {
		report.Summary.Combined = float64(combinedCovered) / float64(combinedTotal) * 100
	}
}

// Normalize applies normalization transformations to the coverage report
// This modifies the report in-place to merge/collapse metrics as specified
func (report *Report) Normalize(config *NormalizationConfig) {
	if config == nil || len(config.Modes) == 0 {
		return
	}

	report.Summary.Normalized = true

	// Apply conditions-to-branches: merge condition counts into branch counts
	if config.ConditionsToBranch {
		report.Summary.ConditionsAbsorbed = true
		for _, fc := range report.Files {
			// Add condition counts to branch counts
			fc.Branches.Total += fc.Conditions.Total
			fc.Branches.Covered += fc.Conditions.Covered
			if fc.Branches.Total > 0 {
				fc.Branches.Percent = float64(fc.Branches.Covered) / float64(fc.Branches.Total) * 100
			}
			// Zero out conditions (they're now in branches)
			fc.Conditions.Total = 0
			fc.Conditions.Covered = 0
			fc.Conditions.Percent = 0
		}
	}

	// Apply subroutines-to-statements: merge subroutine counts into statement counts
	if config.SubroutinesToStmt {
		report.Summary.SubroutinesAbsorbed = true
		for _, fc := range report.Files {
			// Add subroutine counts to statement counts
			fc.Statements.Total += fc.Subroutines.Total
			fc.Statements.Covered += fc.Subroutines.Covered
			if fc.Statements.Total > 0 {
				fc.Statements.Percent = float64(fc.Statements.Covered) / float64(fc.Statements.Total) * 100
			}
			// Zero out subroutines (they're now in statements)
			fc.Subroutines.Total = 0
			fc.Subroutines.Covered = 0
			fc.Subroutines.Percent = 0
		}
	}

	// Simple mode: collapse everything to just statements
	if config.SimpleMode {
		report.Summary.ConditionsAbsorbed = true
		report.Summary.SubroutinesAbsorbed = true
		for _, fc := range report.Files {
			// Zero out non-statement metrics
			fc.Branches.Total = 0
			fc.Branches.Covered = 0
			fc.Branches.Percent = 0
			fc.Conditions.Total = 0
			fc.Conditions.Covered = 0
			fc.Conditions.Percent = 0
			fc.Subroutines.Total = 0
			fc.Subroutines.Covered = 0
			fc.Subroutines.Percent = 0
		}
	}

	// Recalculate summary after normalization
	report.recalculateSummary()
}

// recalculateSummary recalculates summary percentages after normalization
func (report *Report) recalculateSummary() {
	var totalStmt, coveredStmt int
	var totalBranch, coveredBranch int
	var totalCond, coveredCond int
	var totalSub, coveredSub int

	for _, fc := range report.Files {
		totalStmt += fc.Statements.Total
		coveredStmt += fc.Statements.Covered
		totalBranch += fc.Branches.Total
		coveredBranch += fc.Branches.Covered
		totalCond += fc.Conditions.Total
		coveredCond += fc.Conditions.Covered
		totalSub += fc.Subroutines.Total
		coveredSub += fc.Subroutines.Covered
	}

	report.Summary.Statement = 0
	report.Summary.Branch = 0
	report.Summary.Condition = 0
	report.Summary.Subroutine = 0
	report.Summary.Combined = 0

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

	// Recalculate combined
	combinedTotal := totalCond + totalStmt
	combinedCovered := coveredCond + coveredStmt
	if combinedTotal > 0 {
		report.Summary.Combined = float64(combinedCovered) / float64(combinedTotal) * 100
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

	// Determine which columns to show based on normalization
	showCond := !report.Summary.ConditionsAbsorbed
	showSub := !report.Summary.SubroutinesAbsorbed
	showCombined := report.Summary.Normalized && report.Summary.Combined > 0

	// Print normalization note if active
	if report.Summary.Normalized {
		fmt.Print("\n[normalized: ")
		var notes []string
		if report.Summary.ConditionsAbsorbed {
			notes = append(notes, "conditions→branches")
		}
		if report.Summary.SubroutinesAbsorbed {
			notes = append(notes, "subroutines→statements")
		}
		fmt.Print(strings.Join(notes, ", "))
		fmt.Println("]")
	}

	// Build header based on active columns
	if showCond && showSub {
		fmt.Printf("\n%-60s %10s %10s %10s %10s\n",
			"File", "Stmt", "Branch", "Cond", "Sub")
		fmt.Println(strings.Repeat("-", 104))
	} else if showCond {
		fmt.Printf("\n%-60s %10s %10s %10s\n",
			"File", "Stmt", "Branch", "Cond")
		fmt.Println(strings.Repeat("-", 94))
	} else if showSub {
		fmt.Printf("\n%-60s %10s %10s %10s\n",
			"File", "Stmt", "Branch", "Sub")
		fmt.Println(strings.Repeat("-", 94))
	} else {
		// Minimal: just Stmt and Branch
		fmt.Printf("\n%-60s %10s %10s\n",
			"File", "Stmt", "Branch")
		fmt.Println(strings.Repeat("-", 84))
	}

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

		if showCond && showSub {
			fmt.Printf("%-60s %10s %10s %10s %10s\n",
				displayPath, stmtStr, branchStr, condStr, subStr)
		} else if showCond {
			fmt.Printf("%-60s %10s %10s %10s\n",
				displayPath, stmtStr, branchStr, condStr)
		} else if showSub {
			fmt.Printf("%-60s %10s %10s %10s\n",
				displayPath, stmtStr, branchStr, subStr)
		} else {
			fmt.Printf("%-60s %10s %10s\n",
				displayPath, stmtStr, branchStr)
		}

		// Show uncovered lines in verbose mode
		if verbose && len(f.Statements.Uncovered) > 0 {
			fmt.Printf("    Uncovered lines: %v\n", f.Statements.Uncovered)
		}
	}

	// Print summary
	if showCond && showSub {
		fmt.Println(strings.Repeat("-", 104))
		fmt.Printf("%-60s %9.1f%% %9.1f%% %9.1f%% %9.1f%%\n",
			"Total",
			report.Summary.Statement,
			report.Summary.Branch,
			report.Summary.Condition,
			report.Summary.Subroutine)
	} else if showCond {
		fmt.Println(strings.Repeat("-", 94))
		fmt.Printf("%-60s %9.1f%% %9.1f%% %9.1f%%\n",
			"Total",
			report.Summary.Statement,
			report.Summary.Branch,
			report.Summary.Condition)
	} else if showSub {
		fmt.Println(strings.Repeat("-", 94))
		fmt.Printf("%-60s %9.1f%% %9.1f%% %9.1f%%\n",
			"Total",
			report.Summary.Statement,
			report.Summary.Branch,
			report.Summary.Subroutine)
	} else {
		fmt.Println(strings.Repeat("-", 84))
		fmt.Printf("%-60s %9.1f%% %9.1f%%\n",
			"Total",
			report.Summary.Statement,
			report.Summary.Branch)
	}

	// Show combined coverage for SonarQube mode
	if showCombined {
		fmt.Printf("\nCombined coverage (SonarQube-style): %.1f%%\n", report.Summary.Combined)
	}
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
