package runtime

import (
	"fmt"
	"time"
)

const (
	LATTICE_RETRY = "LATTICE_RETRY"
)

// Retry errors caused by Lattice APIs which need high requeueAfter seconds
func NewRetryError() *RequeueNeededAfter {
	return NewRequeueNeededAfter(
		LATTICE_RETRY,
		time.Second*10,
	)
}

// NewRequeueNeededAfter constructs new RequeueNeededAfter to
// instruct controller-runtime to requeue the processing item after specified duration without been logged as error.
func NewRequeueNeededAfter(reason string, duration time.Duration) *RequeueNeededAfter {
	return &RequeueNeededAfter{
		reason:   reason,
		duration: duration,
	}
}

var _ error = &RequeueNeededAfter{}

// An error to instruct controller-runtime to requeue the processing item after specified duration without been logged as error.
// This should be used when a "error condition" occurrence is sort of expected and can be resolved by retry.
// e.g. a dependency haven't been fulfilled yet, and expected it to be fulfilled after duration.
// Note: use this with care,a simple wait might suits your use case better.
type RequeueNeededAfter struct {
	reason   string
	duration time.Duration
}

func (e *RequeueNeededAfter) Reason() string {
	return e.reason
}

func (e *RequeueNeededAfter) Duration() time.Duration {
	return e.duration
}

func (e *RequeueNeededAfter) Error() string {
	return fmt.Sprintf("requeue needed after %v: %v", e.duration, e.reason)
}
