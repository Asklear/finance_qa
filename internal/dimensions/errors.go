package dimensions

import "errors"

var (
	ErrInvalidInput  = errors.New("dimensions: invalid input")
	ErrAlreadyExists = errors.New("dimensions: already exists")
	ErrNotFound      = errors.New("dimensions: not found")
	ErrConflict      = errors.New("dimensions: conflict")
)
