# Claude Code Bootstrap Guide

This file provides instructions for setting up the development environment and testing perlcov.

## Prerequisites

### Installing Dependencies

```bash
# Install Devel::Cover (required for coverage collection)
apt-get update && apt-get install -y libdevel-cover-perl

# Verify installation
perl -MDevel::Cover -e 'print $Devel::Cover::VERSION, "\n"'
```

### Building perlcov

```bash
# Build the binary
go build -o perlcov ./cmd/perlcov/

# Verify the build
./perlcov --version
```

## Testing with Sample Modules

### Creating a Test Project

```bash
# Create a test project structure
mkdir -p testproject/lib/My/Test testproject/lib/Other testproject/t

# Create a sample Perl module (lib/My/Test/Module.pm)
cat > testproject/lib/My/Test/Module.pm << 'EOF'
package My::Test::Module;
use strict;
use warnings;

sub new { my ($class, %args) = @_; bless \%args, $class }
sub greet { my ($self, $name) = @_; $name ? "Hello, $name!" : "Hello, World!" }
sub add { my ($self, $a, $b) = @_; return $a + $b }

1;
EOF

# Create a matching test file (t/My-Test-Module.t)
# Note: Filename pattern Module-Name.t maps to Module::Name
cat > testproject/t/My-Test-Module.t << 'EOF'
#!/usr/bin/env perl
use strict;
use warnings;
use Test::More tests => 4;

use_ok('My::Test::Module');
my $obj = My::Test::Module->new();
is($obj->greet(), 'Hello, World!', 'greet without name');
is($obj->greet('Alice'), 'Hello, Alice!', 'greet with name');
is($obj->add(2, 3), 5, 'add works');
EOF
```

### Running perlcov

```bash
cd testproject
../perlcov -v -j 1

# The -v flag shows verbose output including which modules are being selected
# Look for lines like: [select] t/My-Test-Module.t -> My::Test::Module
```

## Module Name Pattern

The `-select` optimization extracts module names from test filenames using this pattern:

| Test File | Extracted Module |
|-----------|-----------------|
| `Module-Install-Something.t` | `Module::Install::Something` |
| `Module-Install_specifier.t` | `Module::Install` |
| `Module.t` | `Module` |
| `00-load.t` | (skipped - numbered test) |
| `basic.t` | (skipped - lowercase) |

The module must:
- Start with an uppercase letter
- Use hyphens to separate namespace parts
- Have `.t` extension

## Testing with Real CPAN Modules

To test with a real CPAN module, download one that uses the Module-Name.t naming convention:

```bash
# Example: Download and extract a module
curl -sL "https://cpan.metacpan.org/authors/id/X/XX/AUTHOR/Module-Name-1.00.tar.gz" -o module.tar.gz
tar xzf module.tar.gz
cd Module-Name-1.00
../perlcov -v
```

## Verifying Coverage with cover

The official Devel::Cover `cover` command can verify coverage data:

```bash
# After running perlcov, check with cover
cover -report text cover_db
```

## How the -select Optimization Works

When a test file matches the `Module-Name.t` pattern:

1. perlcov extracts the module name (`Module::Name`)
2. If the module exists in `lib/`, perlcov adds Devel::Cover options:
   - `-ignore,lib/` - exclude all lib files by default
   - `-select,Module/Name` - but include the target module
3. The **order matters**: `-ignore` must come before `-select` for proper filtering

This means:
- `My-Test-Module.t` will only collect coverage for `lib/My/Test/Module.pm`
- Other lib modules won't be instrumented, reducing overhead
- Non-matching tests (numbered, lowercase) run with default coverage behavior

### Disabling the Optimization

For benchmarking purposes, you can disable the `-select` optimization with:

```bash
./perlcov --no-select
```

This runs all tests without the targeted coverage filtering, which is useful for comparing performance with and without the optimization.

## Known Issues

1. **Coverage parsing**: perlcov's internal coverage parser may not correctly read all Devel::Cover database formats. Use the `--html` flag or `cover` command for accurate reports.

## Go Test Commands

```bash
# Run unit tests
go test ./...

# Run tests with verbose output
go test -v ./internal/runner/
```
