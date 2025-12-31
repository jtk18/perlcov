package runner

import "testing"

func TestExtractModuleFromTestFile(t *testing.T) {
	tests := []struct {
		name     string
		testFile string
		expected string
	}{
		{
			name:     "simple module pattern",
			testFile: "Module-Install-Something.t",
			expected: "Module::Install::Something",
		},
		{
			name:     "module pattern with specifier",
			testFile: "Module-Install-Something_specifier_multi.t",
			expected: "Module::Install::Something",
		},
		{
			name:     "module pattern with path",
			testFile: "t/Module-Install-Something.t",
			expected: "Module::Install::Something",
		},
		{
			name:     "module pattern with absolute path",
			testFile: "/home/user/project/t/Module-Install-Something.t",
			expected: "Module::Install::Something",
		},
		{
			name:     "two-part module",
			testFile: "Module-Something.t",
			expected: "Module::Something",
		},
		{
			name:     "deeply nested module",
			testFile: "App-Foo-Bar-Baz-Qux.t",
			expected: "App::Foo::Bar::Baz::Qux",
		},
		{
			name:     "single word module",
			testFile: "Module.t",
			expected: "Module",
		},
		{
			name:     "single word module with specifier",
			testFile: "Module_specifier.t",
			expected: "Module",
		},
		{
			name:     "non-matching simple test",
			testFile: "basic.t",
			expected: "",
		},
		{
			name:     "non-matching numbered test",
			testFile: "00-load.t",
			expected: "",
		},
		{
			name:     "non-matching numbered test with multiple digits",
			testFile: "123-some-test.t",
			expected: "",
		},
		{
			name:     "non-matching lowercase hyphenated",
			testFile: "my-test-file.t",
			expected: "",
		},
		{
			name:     "non-matching with underscore only",
			testFile: "basic_test.t",
			expected: "",
		},
		{
			name:     "wrong extension",
			testFile: "Module-Something.pm",
			expected: "",
		},
		{
			name:     "empty filename",
			testFile: ".t",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModuleFromTestFile(tt.testFile)
			if result != tt.expected {
				t.Errorf("extractModuleFromTestFile(%q) = %q, want %q", tt.testFile, result, tt.expected)
			}
		})
	}
}

func TestContainsTAPFailure(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "all tests pass",
			output:   "1..3\nok 1 - first test\nok 2 - second test\nok 3 - third test\n",
			expected: false,
		},
		{
			name:     "simple failure",
			output:   "1..2\nok 1 - first test\nnot ok 2 - second test\n",
			expected: true,
		},
		{
			name:     "TODO test not a failure",
			output:   "1..2\nok 1 - first test\nnot ok 2 - pending feature # TODO\n",
			expected: false,
		},
		{
			name:     "SKIP test not a failure",
			output:   "1..2\nok 1 - first test\nnot ok 2 - optional feature # SKIP\n",
			expected: false,
		},
		{
			name:     "bail out",
			output:   "1..5\nok 1 - first test\nBail out! Something went very wrong\n",
			expected: true,
		},
		{
			name:     "empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "only plan",
			output:   "1..0\n",
			expected: false,
		},
		{
			name:     "not ok in middle of line is not failure",
			output:   "# this is not ok to do\nok 1 - test\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsTAPFailure(tt.output)
			if result != tt.expected {
				t.Errorf("containsTAPFailure(%q) = %v, want %v", tt.output, result, tt.expected)
			}
		})
	}
}

func TestNewRunner(t *testing.T) {
	r := New([]string{"/path/to/lib"}, "/cover/dir", 4, true, []string{"lib", "src"}, true, false, "/usr/bin/perl")

	if len(r.IncludePaths) != 1 || r.IncludePaths[0] != "/path/to/lib" {
		t.Errorf("IncludePaths = %v, want [/path/to/lib]", r.IncludePaths)
	}
	if r.CoverDir != "/cover/dir" {
		t.Errorf("CoverDir = %q, want /cover/dir", r.CoverDir)
	}
	if r.Jobs != 4 {
		t.Errorf("Jobs = %d, want 4", r.Jobs)
	}
	if !r.Verbose {
		t.Error("Verbose = false, want true")
	}
	if len(r.SourceDirs) != 2 {
		t.Errorf("SourceDirs = %v, want [lib src]", r.SourceDirs)
	}
	if !r.NoSelect {
		t.Error("NoSelect = false, want true")
	}
	if r.PerlPath != "/usr/bin/perl" {
		t.Errorf("PerlPath = %q, want /usr/bin/perl", r.PerlPath)
	}
}
