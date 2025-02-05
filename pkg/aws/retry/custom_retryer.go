package retry

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"math"
	"math/rand"
	"time"
)

const (
	BackoffMultiplier = 100
)

func WithMaxRetries(maxRetries int) request.Option {
	return func(r *request.Request) {
		r.Retryer = &CustomRetryer{
			Retryer:       r.Retryer,
			numMaxRetries: maxRetries,
		}
	}
}

type CustomRetryer struct {
	request.Retryer
	numMaxRetries int
}

func (c *CustomRetryer) MaxRetries() int {
	return c.numMaxRetries
}

func (c *CustomRetryer) RetryRules(req *request.Request) time.Duration {
	if req.RetryCount < c.numMaxRetries {
		backoff := time.Duration((math.Pow(2, float64(req.RetryCount)))*BackoffMultiplier) * time.Millisecond
		jitter := time.Duration(float64(backoff) * (0.1*rand.Float64() - 0.05))
		return backoff + jitter
	}
	return 0
}
