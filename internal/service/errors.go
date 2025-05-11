// errors.go
package service

import "errors"

var (
	ErrClosed       = errors.New("broker is closed")
	ErrBufferFull   = errors.New("subscriber buffer full")
	ErrInvalidInput = errors.New("invalid subscribe parameters")
)
