package txn

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestIsNotSupported(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  errors.New("some random error"),
			want: false,
		},
		{
			name: "command error code 20",
			err:  mongo.CommandError{Code: 20, Message: "Transaction numbers are only allowed on a replica set member"},
			want: true,
		},
		{
			name: "command error code 51",
			err:  mongo.CommandError{Code: 51, Message: "Illegal operation"},
			want: true,
		},
		{
			name: "command error code 263",
			err:  mongo.CommandError{Code: 263, Message: "Cannot run in a multi-document transaction"},
			want: true,
		},
		{
			name: "other command error code",
			err:  mongo.CommandError{Code: 100, Message: "Some other error"},
			want: false,
		},
		{
			name: "error with transaction and replica set keywords",
			err:  errors.New("transaction failed because this is not a replica set member"),
			want: true,
		},
		{
			name: "error with session and not supported keywords",
			err:  errors.New("session operations are not supported on this server"),
			want: true,
		},
		{
			name: "error with only one keyword",
			err:  errors.New("transaction failed"),
			want: false,
		},
		{
			name: "error with transaction and session",
			err:  errors.New("cannot start transaction in current session state"),
			want: true,
		},
		{
			name: "error with illegal operation keywords",
			err:  errors.New("illegal operation during transaction"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotSupported(tt.err)
			if got != tt.want {
				t.Errorf("IsNotSupported(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsNotSupported_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "uppercase TRANSACTION and REPLICA SET",
			err:  errors.New("TRANSACTION FAILED on REPLICA SET"),
			want: true,
		},
		{
			name: "mixed case Transaction and Session",
			err:  errors.New("Transaction Session error"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNotSupported(tt.err)
			if got != tt.want {
				t.Errorf("IsNotSupported(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
