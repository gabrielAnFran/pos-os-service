package repositories

import "errors"

// ErrNotFound is returned by repository lookups that find nothing.
var ErrNotFound = errors.New("resource not found")
