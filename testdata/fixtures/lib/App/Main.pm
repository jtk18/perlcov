package App::Main;
use strict;
use warnings;

sub new {
    my ($class, %args) = @_;
    bless \%args, $class;
}

sub run {
    my ($self) = @_;
    if ($self->{verbose}) {
        print "Running in verbose mode\n";
    } else {
        print "Running in quiet mode\n";
    }
    return 1;
}

sub process {
    my ($self, $data) = @_;
    return unless $data;

    if ($data->{type} eq 'a' && $data->{valid}) {
        return $self->handle_type_a($data);
    } elsif ($data->{type} eq 'b' || $data->{fallback}) {
        return $self->handle_type_b($data);
    }
    return 0;
}

sub handle_type_a {
    my ($self, $data) = @_;
    return $data->{value} * 2;
}

sub handle_type_b {
    my ($self, $data) = @_;
    return $data->{value} + 10;
}

sub unused_method {
    my ($self) = @_;
    # This should show as uncovered
    return "never called";
}

1;
