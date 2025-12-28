#!/usr/bin/env perl
use strict;
use warnings;
use Test::More tests => 3;

# Numbered test - should NOT trigger -select optimization
use_ok('App::Main');
use_ok('App::Helper');
use_ok('App::Utils');
