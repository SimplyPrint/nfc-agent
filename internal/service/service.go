package service

import "errors"

// Common errors
var (
	ErrNotInstalled = errors.New("service is not installed")
	ErrAlreadyInstalled = errors.New("service is already installed")
)

// Service represents a platform-specific auto-start service
type Service interface {
	Install() error
	Uninstall() error
	IsInstalled() bool
	Status() (string, error)
}
