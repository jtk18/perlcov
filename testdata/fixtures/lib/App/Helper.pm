package App::Helper;
use strict;
use warnings;

sub new {
    my ($class) = @_;
    bless {}, $class;
}

sub format_name {
    my ($self, $first, $last) = @_;
    return "$first $last" if $first && $last;
    return $first if $first;
    return $last if $last;
    return "Anonymous";
}

sub validate {
    my ($self, $value) = @_;
    return 0 unless defined $value;
    return 0 if $value < 0;
    return 0 if $value > 100;
    return 1;
}

1;
