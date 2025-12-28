#!/usr/bin/env perl
use strict;
use warnings;
use Test::More tests => 5;

use_ok('App::Main');

my $app = App::Main->new(verbose => 1);
isa_ok($app, 'App::Main');

ok($app->run(), 'run returns true');

my $result = $app->process({ type => 'a', valid => 1, value => 5 });
is($result, 10, 'process type a doubles value');

$result = $app->process({ type => 'b', value => 5 });
is($result, 15, 'process type b adds 10');
