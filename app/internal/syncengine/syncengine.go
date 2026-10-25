// Package syncengine defines how files reach a Sync Engine's Destination.
package syncengine

import (
	"context"
	"errors"
)

var (
	// ErrUnauthorized means the Owner's credentials for the Sync Engine were rejected or revoked.
	ErrUnauthorized = errors.New("sync engine authorization rejected")
	// ErrDestinationNotFound means the configured Destination folder does not exist on the Sync Engine or is not shared with this app.
	ErrDestinationNotFound = errors.New("destination not found")
)

// Engine stores files in a single Destination on a Sync Engine.
type Engine interface {
	// Name is the Destination's display name, e.g. "INBOX".
	Name() string
	Exists(ctx context.Context, name string) (bool, error)
	Put(ctx context.Context, name string, content []byte) error
}
