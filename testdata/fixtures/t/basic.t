#!/usr/bin/env perl
use strict;
use warnings;
use Test::More tests => 2;

# Lowercase test - should NOT trigger -select optimization
use_ok('App::Main');
my $app = App::Main->new();
ok($app->run(), 'basic run');
