package coverage

import "testing"

func TestParseNormalizationModes(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantCondToBranch bool
		wantSubToStmt    bool
		wantSonarQube    bool
		wantSimple       bool
		wantErr          bool
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:             "conditions-to-branches",
			input:            "conditions-to-branches",
			wantCondToBranch: true,
		},
		{
			name:          "subroutines-to-statements",
			input:         "subroutines-to-statements",
			wantSubToStmt: true,
		},
		{
			name:             "sonarqube enables conditions-to-branches",
			input:            "sonarqube",
			wantSonarQube:    true,
			wantCondToBranch: true,
		},
		{
			name:       "simple mode",
			input:      "simple",
			wantSimple: true,
		},
		{
			name:             "multiple modes comma-separated",
			input:            "conditions-to-branches,subroutines-to-statements",
			wantCondToBranch: true,
			wantSubToStmt:    true,
		},
		{
			name:             "multiple modes with spaces",
			input:            "conditions-to-branches, subroutines-to-statements",
			wantCondToBranch: true,
			wantSubToStmt:    true,
		},
		{
			name:    "unknown mode",
			input:   "unknown-mode",
			wantErr: true,
		},
		{
			name:    "partially valid",
			input:   "conditions-to-branches,invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseNormalizationModes(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseNormalizationModes(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseNormalizationModes(%q) unexpected error: %v", tt.input, err)
				return
			}

			if config.ConditionsToBranch != tt.wantCondToBranch {
				t.Errorf("ConditionsToBranch = %v, want %v", config.ConditionsToBranch, tt.wantCondToBranch)
			}
			if config.SubroutinesToStmt != tt.wantSubToStmt {
				t.Errorf("SubroutinesToStmt = %v, want %v", config.SubroutinesToStmt, tt.wantSubToStmt)
			}
			if config.SonarQubeStyle != tt.wantSonarQube {
				t.Errorf("SonarQubeStyle = %v, want %v", config.SonarQubeStyle, tt.wantSonarQube)
			}
			if config.SimpleMode != tt.wantSimple {
				t.Errorf("SimpleMode = %v, want %v", config.SimpleMode, tt.wantSimple)
			}
		})
	}
}

func TestNormalize_ConditionsToBranches(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Foo.pm": {
				Path: "lib/Foo.pm",
				Statements: StatementCoverage{
					Covered: 10,
					Total:   20,
					Percent: 50.0,
				},
				Branches: BranchCoverage{
					Covered: 5,
					Total:   10,
					Percent: 50.0,
				},
				Conditions: ConditionCoverage{
					Covered: 3,
					Total:   6,
					Percent: 50.0,
				},
				Subroutines: SubroutineCoverage{
					Covered: 2,
					Total:   4,
					Percent: 50.0,
				},
			},
		},
	}

	config := &NormalizationConfig{
		ConditionsToBranch: true,
		Modes:              []NormalizationMode{NormalizeConditionsToBranches},
	}

	report.Normalize(config)

	fc := report.Files["lib/Foo.pm"]

	// Branches should now include conditions
	if fc.Branches.Total != 16 { // 10 + 6
		t.Errorf("Branches.Total = %d, want 16", fc.Branches.Total)
	}
	if fc.Branches.Covered != 8 { // 5 + 3
		t.Errorf("Branches.Covered = %d, want 8", fc.Branches.Covered)
	}

	// Conditions should be zeroed
	if fc.Conditions.Total != 0 {
		t.Errorf("Conditions.Total = %d, want 0", fc.Conditions.Total)
	}
	if fc.Conditions.Covered != 0 {
		t.Errorf("Conditions.Covered = %d, want 0", fc.Conditions.Covered)
	}

	// Summary should reflect absorption
	if !report.Summary.ConditionsAbsorbed {
		t.Error("ConditionsAbsorbed = false, want true")
	}
}

func TestNormalize_SubroutinesToStatements(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Bar.pm": {
				Path: "lib/Bar.pm",
				Statements: StatementCoverage{
					Covered: 10,
					Total:   20,
					Percent: 50.0,
					lines:   map[int]int{1: 1, 2: 0},
				},
				Branches: BranchCoverage{
					Covered: 5,
					Total:   10,
					Percent: 50.0,
				},
				Conditions: ConditionCoverage{
					Covered: 3,
					Total:   6,
					Percent: 50.0,
				},
				Subroutines: SubroutineCoverage{
					Covered: 4,
					Total:   8,
					Percent: 50.0,
				},
			},
		},
	}

	config := &NormalizationConfig{
		SubroutinesToStmt: true,
		Modes:             []NormalizationMode{NormalizeSubroutinesToStatements},
	}

	report.Normalize(config)

	fc := report.Files["lib/Bar.pm"]

	// Statements should now include subroutines
	if fc.Statements.Total != 28 { // 20 + 8
		t.Errorf("Statements.Total = %d, want 28", fc.Statements.Total)
	}
	if fc.Statements.Covered != 14 { // 10 + 4
		t.Errorf("Statements.Covered = %d, want 14", fc.Statements.Covered)
	}

	// Subroutines should be zeroed
	if fc.Subroutines.Total != 0 {
		t.Errorf("Subroutines.Total = %d, want 0", fc.Subroutines.Total)
	}
	if fc.Subroutines.Covered != 0 {
		t.Errorf("Subroutines.Covered = %d, want 0", fc.Subroutines.Covered)
	}

	// Summary should reflect absorption
	if !report.Summary.SubroutinesAbsorbed {
		t.Error("SubroutinesAbsorbed = false, want true")
	}
}

func TestNormalize_SimpleMode(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Baz.pm": {
				Path: "lib/Baz.pm",
				Statements: StatementCoverage{
					Covered: 15,
					Total:   30,
					Percent: 50.0,
					lines:   map[int]int{1: 1},
				},
				Branches: BranchCoverage{
					Covered: 5,
					Total:   10,
					Percent: 50.0,
				},
				Conditions: ConditionCoverage{
					Covered: 3,
					Total:   6,
					Percent: 50.0,
				},
				Subroutines: SubroutineCoverage{
					Covered: 2,
					Total:   4,
					Percent: 50.0,
				},
			},
		},
	}

	config := &NormalizationConfig{
		SimpleMode: true,
		Modes:      []NormalizationMode{NormalizeSimple},
	}

	report.Normalize(config)

	fc := report.Files["lib/Baz.pm"]

	// Statements should remain unchanged
	if fc.Statements.Total != 30 {
		t.Errorf("Statements.Total = %d, want 30", fc.Statements.Total)
	}

	// All other metrics should be zeroed
	if fc.Branches.Total != 0 {
		t.Errorf("Branches.Total = %d, want 0", fc.Branches.Total)
	}
	if fc.Conditions.Total != 0 {
		t.Errorf("Conditions.Total = %d, want 0", fc.Conditions.Total)
	}
	if fc.Subroutines.Total != 0 {
		t.Errorf("Subroutines.Total = %d, want 0", fc.Subroutines.Total)
	}
}

func TestNormalize_SonarQubeStyle(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Qux.pm": {
				Path: "lib/Qux.pm",
				Statements: StatementCoverage{
					Covered: 10,
					Total:   20,
					Percent: 50.0,
					lines:   map[int]int{1: 1},
				},
				Branches: BranchCoverage{
					Covered: 5,
					Total:   10,
					Percent: 50.0,
				},
				Conditions: ConditionCoverage{
					Covered: 3,
					Total:   6,
					Percent: 50.0,
				},
				Subroutines: SubroutineCoverage{
					Covered: 2,
					Total:   4,
					Percent: 50.0,
				},
			},
		},
	}

	config := &NormalizationConfig{
		SonarQubeStyle:     true,
		ConditionsToBranch: true, // SonarQube implies this
		Modes:              []NormalizationMode{NormalizeSonarQube},
	}

	report.Normalize(config)

	fc := report.Files["lib/Qux.pm"]

	// Conditions should be absorbed into branches
	if fc.Branches.Total != 16 { // 10 + 6
		t.Errorf("Branches.Total = %d, want 16", fc.Branches.Total)
	}
	if fc.Conditions.Total != 0 {
		t.Errorf("Conditions.Total = %d, want 0", fc.Conditions.Total)
	}

	// Summary should be marked as normalized
	if !report.Summary.Normalized {
		t.Error("Normalized = false, want true")
	}
}

func TestNormalize_NilConfig(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Test.pm": {
				Path: "lib/Test.pm",
				Branches: BranchCoverage{
					Total: 10,
				},
				Conditions: ConditionCoverage{
					Total: 6,
				},
			},
		},
	}

	// Should not panic and should not modify
	report.Normalize(nil)

	fc := report.Files["lib/Test.pm"]
	if fc.Branches.Total != 10 {
		t.Errorf("Branches.Total = %d, want 10 (unchanged)", fc.Branches.Total)
	}
}

func TestNormalize_EmptyConfig(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Test.pm": {
				Path: "lib/Test.pm",
				Branches: BranchCoverage{
					Total: 10,
				},
				Conditions: ConditionCoverage{
					Total: 6,
				},
			},
		},
	}

	config := &NormalizationConfig{}

	// Should not modify with empty config
	report.Normalize(config)

	fc := report.Files["lib/Test.pm"]
	if fc.Branches.Total != 10 {
		t.Errorf("Branches.Total = %d, want 10 (unchanged)", fc.Branches.Total)
	}
}

func TestFormatCoverage(t *testing.T) {
	tests := []struct {
		name    string
		covered int
		total   int
		want    string
	}{
		{"zero total", 0, 0, "n/a"},
		{"100 percent", 10, 10, "100.0%"},
		{"50 percent", 5, 10, "50.0%"},
		{"0 percent", 0, 10, "0.0%"},
		{"partial", 3, 7, "42.9%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCoverage(tt.covered, tt.total)
			if got != tt.want {
				t.Errorf("formatCoverage(%d, %d) = %q, want %q", tt.covered, tt.total, got, tt.want)
			}
		})
	}
}

func TestCalculateSummary(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/A.pm": {
				Path: "lib/A.pm",
				Statements: StatementCoverage{
					Covered: 10,
					Total:   20,
					lines:   map[int]int{5: 0, 10: 0},
				},
				Branches: BranchCoverage{
					Covered: 4,
					Total:   8,
				},
				Conditions: ConditionCoverage{
					Covered: 2,
					Total:   6,
				},
				Subroutines: SubroutineCoverage{
					Covered: 3,
					Total:   4,
				},
			},
			"lib/B.pm": {
				Path: "lib/B.pm",
				Statements: StatementCoverage{
					Covered: 15,
					Total:   20,
					lines:   map[int]int{},
				},
				Branches: BranchCoverage{
					Covered: 6,
					Total:   8,
				},
				Conditions: ConditionCoverage{
					Covered: 4,
					Total:   6,
				},
				Subroutines: SubroutineCoverage{
					Covered: 4,
					Total:   4,
				},
			},
		},
	}

	calculateSummary(report)

	// Check individual file percentages
	fcA := report.Files["lib/A.pm"]
	if fcA.Statements.Percent != 50.0 {
		t.Errorf("A.Statements.Percent = %f, want 50.0", fcA.Statements.Percent)
	}

	// Check summary totals (10+15=25 covered, 20+20=40 total = 62.5%)
	if report.Summary.Statement != 62.5 {
		t.Errorf("Summary.Statement = %f, want 62.5", report.Summary.Statement)
	}

	// Check branch summary (4+6=10 covered, 8+8=16 total = 62.5%)
	if report.Summary.Branch != 62.5 {
		t.Errorf("Summary.Branch = %f, want 62.5", report.Summary.Branch)
	}

	// Check file counts
	if report.Summary.TotalFiles != 2 {
		t.Errorf("Summary.TotalFiles = %d, want 2", report.Summary.TotalFiles)
	}
	if report.Summary.CoveredFiles != 2 {
		t.Errorf("Summary.CoveredFiles = %d, want 2", report.Summary.CoveredFiles)
	}

	// Check uncovered lines were built correctly
	if len(fcA.Statements.Uncovered) != 2 {
		t.Errorf("A.Statements.Uncovered length = %d, want 2", len(fcA.Statements.Uncovered))
	}
}

func TestNormalize_CombinedModes(t *testing.T) {
	report := &Report{
		Files: map[string]*FileCoverage{
			"lib/Test.pm": {
				Path: "lib/Test.pm",
				Statements: StatementCoverage{
					Covered: 10,
					Total:   20,
				},
				Branches: BranchCoverage{
					Covered: 5,
					Total:   10,
				},
				Conditions: ConditionCoverage{
					Covered: 3,
					Total:   6,
				},
				Subroutines: SubroutineCoverage{
					Covered: 2,
					Total:   4,
				},
			},
		},
	}

	config := &NormalizationConfig{
		ConditionsToBranch: true,
		SubroutinesToStmt:  true,
		Modes: []NormalizationMode{
			NormalizeConditionsToBranches,
			NormalizeSubroutinesToStatements,
		},
	}

	report.Normalize(config)

	fc := report.Files["lib/Test.pm"]

	// Both conditions and subroutines should be absorbed
	if fc.Branches.Total != 16 { // 10 + 6
		t.Errorf("Branches.Total = %d, want 16", fc.Branches.Total)
	}
	if fc.Statements.Total != 24 { // 20 + 4
		t.Errorf("Statements.Total = %d, want 24", fc.Statements.Total)
	}
	if fc.Conditions.Total != 0 {
		t.Errorf("Conditions.Total = %d, want 0", fc.Conditions.Total)
	}
	if fc.Subroutines.Total != 0 {
		t.Errorf("Subroutines.Total = %d, want 0", fc.Subroutines.Total)
	}

	// Summary should reflect both absorptions
	if !report.Summary.ConditionsAbsorbed {
		t.Error("ConditionsAbsorbed = false, want true")
	}
	if !report.Summary.SubroutinesAbsorbed {
		t.Error("SubroutinesAbsorbed = false, want true")
	}
}
