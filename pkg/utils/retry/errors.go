package retry

// Retriable is the retry interface
type Retriable interface {
	Retry() bool
}

// DefaultRetriable is the default retryable
type DefaultRetriable struct {
	retry bool
}

// Retry does the retry
func (dr DefaultRetriable) Retry() bool {
	return dr.retry
}

// NewRetriable creates a new Retriable
func NewRetriable(retry bool) Retriable {
	return DefaultRetriable{
		retry: retry,
	}
}

// RetriableError interface
type RetriableError interface {
	Retriable
	error
}

// DefaultRetriableError is the default retriable error
type DefaultRetriableError struct {
	Retriable
	error
}

// NewRetriableError returns a new retriable error
func NewRetriableError(retriable Retriable, err error) RetriableError {
	return &DefaultRetriableError{
		retriable,
		err,
	}
}
