package lattice

import "errors"

const (
	LATTICE_RETRY = "LATTICE_RETRY"
)

var RetryErr = errors.New(LATTICE_RETRY)
