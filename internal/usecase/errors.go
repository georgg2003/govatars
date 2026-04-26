package usecase

import "errors"

var (
	ErrNotFound        = errors.New("resource not found")
	ErrForbidden       = errors.New("forbidden")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrInvalidImage    = errors.New("invalid image type")
	ErrObjectNotFound  = errors.New("object not found")
)
