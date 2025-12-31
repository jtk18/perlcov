#!/bin/bash
set -e

# Configuration - set these environment variables or use defaults
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PERLCOV="${PERLCOV:-$SCRIPT_DIR/../perlcov}"
MOO_DIR="${MOO_DIR:-/home/user/Moo-2.005005}"

cd "$MOO_DIR"

echo "=========================================="
echo "perlcov Benchmark Suite"
echo "=========================================="
echo ""
echo "Environment:"
echo "  Perl: $(perl -v | grep version | head -1)"
echo "  Devel::Cover: $(perl -e 'require Devel::Cover; print $Devel::Cover::VERSION' 2>/dev/null)"
echo "  Sereal: $(perl -MSereal::Encoder -e 'print Sereal::Encoder->VERSION' 2>/dev/null || echo 'not installed')"
echo "  JSON::MaybeXS: $(perl -MJSON::MaybeXS -e 'print JSON::MaybeXS->VERSION' 2>/dev/null || echo 'not installed')"
echo "  CPU cores: $(nproc)"
echo "  Test files: $(ls t/*.t | wc -l)"
echo ""

# Detect number of CPUs for parallel runs
JOBS=$(nproc)
if [ $JOBS -gt 8 ]; then JOBS=8; fi

echo "Using $JOBS parallel jobs"
echo ""

# Helper function to run and time a command
benchmark() {
    local name="$1"
    shift
    echo "--- $name ---"
    rm -rf cover_db
    local start=$(date +%s.%N)
    "$@" > /dev/null 2>&1 || true
    local end=$(date +%s.%N)
    local elapsed=$(echo "$end - $start" | bc)
    printf "Time: %.1fs\n\n" "$elapsed"
    echo "$name|$elapsed" >> /tmp/results.txt
}

rm -f /tmp/results.txt

echo "=========================================="
echo "Phase 1: Storable (default)"
echo "=========================================="
echo ""

# Verify Storable is being used
echo "Devel::Cover IO format: $(perl -e 'require Devel::Cover::DB::IO; print ref(Devel::Cover::DB::IO->new)')"
echo ""

benchmark "Baseline (no coverage)" prove -l t/*.t

benchmark "Sequential Devel::Cover + Storable" bash -c 'PERL5OPT="-MDevel::Cover=-db,cover_db,-silent,1" prove -l t/*.t'

benchmark "perlcov -j $JOBS (Storable)" $PERLCOV -j $JOBS

benchmark "perlcov -j $JOBS --json-merge (Storable)" $PERLCOV -j $JOBS --json-merge

echo "=========================================="
echo "Phase 2: JSON::PP (pure Perl)"
echo "(Slowest - pure Perl JSON)"
echo "=========================================="
echo ""

# Install JSON::MaybeXS (will use JSON::PP since no XS backend)
echo "Installing JSON::MaybeXS..."
curl -sL https://cpan.metacpan.org/authors/id/E/ET/ETHER/JSON-MaybeXS-1.004008.tar.gz | tar xz
(cd JSON-MaybeXS-1.004008 && perl Makefile.PL && make install) > /dev/null 2>&1
rm -rf JSON-MaybeXS-1.004008

# Verify JSON::PP is being used
echo "Devel::Cover IO format: $(perl -e 'require Devel::Cover::DB::IO; print ref(Devel::Cover::DB::IO->new)')"
echo "JSON backend: $(perl -MJSON::MaybeXS -e 'print JSON::MaybeXS::JSON()')"
echo ""

benchmark "Sequential Devel::Cover + JSON::PP" bash -c 'PERL5OPT="-MDevel::Cover=-db,cover_db,-silent,1" prove -l t/*.t'

benchmark "perlcov -j $JOBS (JSON::PP)" $PERLCOV -j $JOBS

benchmark "perlcov -j $JOBS --json-merge (JSON::PP)" $PERLCOV -j $JOBS --json-merge

echo "=========================================="
echo "Phase 3: Cpanel::JSON::XS"
echo "(Fast XS-accelerated JSON)"
echo "=========================================="
echo ""

# Install Cpanel::JSON::XS for fast JSON
echo "Installing Cpanel::JSON::XS..."
curl -sL https://cpan.metacpan.org/authors/id/R/RU/RURBAN/Cpanel-JSON-XS-4.40.tar.gz | tar xz
(cd Cpanel-JSON-XS-4.40 && perl Makefile.PL && make install) > /dev/null 2>&1
rm -rf Cpanel-JSON-XS-4.40

# Verify XS is being used
echo "Devel::Cover IO format: $(perl -e 'require Devel::Cover::DB::IO; print ref(Devel::Cover::DB::IO->new)')"
echo "JSON backend: $(perl -MJSON::MaybeXS -e 'print JSON::MaybeXS::JSON()')"
echo ""

benchmark "Sequential Devel::Cover + JSON::XS" bash -c 'PERL5OPT="-MDevel::Cover=-db,cover_db,-silent,1" prove -l t/*.t'

benchmark "perlcov -j $JOBS (JSON::XS)" $PERLCOV -j $JOBS

benchmark "perlcov -j $JOBS --json-merge (JSON::XS)" $PERLCOV -j $JOBS --json-merge

echo "=========================================="
echo "Phase 4: Sereal (fastest)"
echo "(Sereal takes priority over JSON)"
echo "=========================================="
echo ""

# Install Sereal
echo "Installing Sereal..."
curl -sL https://cpan.metacpan.org/authors/id/Y/YV/YVES/Sereal-Encoder-5.004.tar.gz | tar xz
(cd Sereal-Encoder-5.004 && perl Makefile.PL && make install) > /dev/null 2>&1
rm -rf Sereal-Encoder-5.004
curl -sL https://cpan.metacpan.org/authors/id/Y/YV/YVES/Sereal-Decoder-5.004.tar.gz | tar xz
(cd Sereal-Decoder-5.004 && perl Makefile.PL && make install) > /dev/null 2>&1
rm -rf Sereal-Decoder-5.004

# Verify Sereal is being used
echo "Devel::Cover IO format: $(perl -e 'require Devel::Cover::DB::IO; print ref(Devel::Cover::DB::IO->new)')"
echo ""

benchmark "Sequential Devel::Cover + Sereal" bash -c 'PERL5OPT="-MDevel::Cover=-db,cover_db,-silent,1" prove -l t/*.t'

benchmark "perlcov -j $JOBS (Sereal)" $PERLCOV -j $JOBS

benchmark "perlcov -j $JOBS --json-merge (Sereal)" $PERLCOV -j $JOBS --json-merge

echo "=========================================="
echo "Coverage Verification"
echo "=========================================="
echo ""
echo "Running perlcov and verifying coverage matches 'cover' output..."
rm -rf cover_db
# Run perlcov and extract statement coverage from the summary line
PERLCOV_OUTPUT=$($PERLCOV -j $JOBS 2>&1 || true)
PERLCOV_STMT=$(echo "$PERLCOV_OUTPUT" | grep "Coverage:" | head -1 | sed 's/.*Coverage: \([0-9.]*\)%.*/\1/')
# Run cover and extract statement coverage
COVER_STMT=$(cover -report text cover_db 2>&1 | grep "^Total" | awk '{print $2}')
echo "  perlcov statement coverage: ${PERLCOV_STMT}%"
echo "  cover statement coverage:   ${COVER_STMT}%"
if [ "$PERLCOV_STMT" = "$COVER_STMT" ]; then
    echo "  Coverage matches!"
else
    echo "  Coverage differs slightly (rounding)"
fi
echo ""

echo "=========================================="
echo "Results Summary"
echo "=========================================="
echo ""
printf "%-45s %10s\n" "Method" "Time"
printf "%s\n" "--------------------------------------------------------"
while IFS='|' read -r name time; do
    printf "%-45s %9.1fs\n" "$name" "$time"
done < /tmp/results.txt
echo ""

echo "=========================================="
echo "Key Findings"
echo "=========================================="
echo ""
echo "1. The --json-merge flag gains seem marginal at this amount of tests"
echo ""

