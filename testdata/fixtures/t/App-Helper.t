#!/usr/bin/env perl
use strict;
use warnings;
use Test::More tests => 7;

use_ok('App::Helper');

my $helper = App::Helper->new();
isa_ok($helper, 'App::Helper');

is($helper->format_name('John', 'Doe'), 'John Doe', 'full name');
is($helper->format_name('John', undef), 'John', 'first only');
is($helper->format_name(undef, 'Doe'), 'Doe', 'last only');
is($helper->format_name(undef, undef), 'Anonymous', 'no name');

ok($helper->validate(50), 'valid value');
