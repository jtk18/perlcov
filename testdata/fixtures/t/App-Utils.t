#!/usr/bin/env perl
use strict;
use warnings;
use Test::More tests => 6;

use_ok('App::Utils');

is(App::Utils::double(5), 10, 'double');
is(App::Utils::triple(3), 9, 'triple');
is(App::Utils::add(2, 3), 5, 'add');

is(App::Utils::complex_logic(1, 1, 1), 'all positive', 'all positive');
is(App::Utils::complex_logic(-1, 1, 1), 'some negative', 'some negative');
