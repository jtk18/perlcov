package App::Utils;
use strict;
use warnings;

sub double { $_[0] * 2 }
sub triple { $_[0] * 3 }
sub add { $_[0] + $_[1] }

sub complex_logic {
    my ($x, $y, $z) = @_;
    if ($x > 0 && $y > 0 && $z > 0) {
        return "all positive";
    } elsif ($x < 0 || $y < 0 || $z < 0) {
        return "some negative";
    } elsif ($x == 0 && $y == 0) {
        return "x and y zero";
    }
    return "other";
}

1;
