package metrics

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/prometheus/client_golang/prometheus"
)

type collector struct {
	instruments *instruments
}

func NewCollector(registerer prometheus.Registerer) (*collector, error) {
	instruments, err := newInstruments(registerer)
	if err != nil {
		return nil, err
	}
	return &collector{
		instruments: instruments,
	}, nil
}

// APIOptions returns middleware stack functions to inject into aws.Config.APIOptions
func (c *collector) APIOptions() []func(*middleware.Stack) error {
	return []func(*middleware.Stack) error{
		func(stack *middleware.Stack) error {
			return stack.Initialize.Add(&apiCallMetricMiddleware{instruments: c.instruments}, middleware.Before)
		},
		func(stack *middleware.Stack) error {
			return stack.Deserialize.Add(&apiRequestMetricMiddleware{instruments: c.instruments}, middleware.Before)
		},
	}
}

// apiCallMetricMiddleware collects per-call metrics (including retries).
// Placed at Initialize (before everything), captures total call duration after next returns.
type apiCallMetricMiddleware struct {
	instruments *instruments
}

func (m *apiCallMetricMiddleware) ID() string { return "collectAPICallMetric" }

func (m *apiCallMetricMiddleware) HandleInitialize(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (
	out middleware.InitializeOutput, metadata middleware.Metadata, err error,
) {
	start := time.Now()
	out, metadata, err = next.HandleInitialize(ctx, in)
	duration := time.Since(start)

	service := middleware.GetServiceID(ctx)
	operation := middleware.GetOperationName(ctx)
	statusCode := "0"
	errorCode := ""

	if err != nil {
		errorCode = errorCodeFromErr(err)
	}
	if results, ok := retry.GetAttemptResults(metadata); ok && len(results.Results) > 0 {
		lastAttempt := results.Results[len(results.Results)-1]
		if resp, ok := lastAttempt.GetRawResponse().(*smithyhttp.Response); ok {
			statusCode = strconv.Itoa(resp.StatusCode)
		}
	}

	m.instruments.apiCallsTotal.With(map[string]string{
		labelService:    service,
		labelOperation:  operation,
		labelStatusCode: statusCode,
		labelErrorCode:  errorCode,
	}).Inc()
	m.instruments.apiCallDurationSeconds.With(map[string]string{
		labelService:   service,
		labelOperation: operation,
	}).Observe(duration.Seconds())

	if results, ok := retry.GetAttemptResults(metadata); ok {
		retries := len(results.Results) - 1
		if retries < 0 {
			retries = 0
		}
		m.instruments.apiCallRetries.With(map[string]string{
			labelService:   service,
			labelOperation: operation,
		}).Observe(float64(retries))
	}

	return out, metadata, err
}

// apiRequestMetricMiddleware collects per-HTTP-request metrics.
// Placed at Deserialize (inside retry loop), captures individual attempt duration.
type apiRequestMetricMiddleware struct {
	instruments *instruments
}

func (m *apiRequestMetricMiddleware) ID() string { return "collectAPIRequestMetric" }

func (m *apiRequestMetricMiddleware) HandleDeserialize(ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	start := time.Now()
	out, metadata, err = next.HandleDeserialize(ctx, in)
	duration := time.Since(start)

	service := middleware.GetServiceID(ctx)
	operation := middleware.GetOperationName(ctx)
	statusCode := "0"
	errorCode := ""

	if resp, ok := out.RawResponse.(*smithyhttp.Response); ok {
		statusCode = strconv.Itoa(resp.StatusCode)
	}
	if err != nil {
		errorCode = errorCodeFromErr(err)
	}

	m.instruments.apiRequestsTotal.With(map[string]string{
		labelService:    service,
		labelOperation:  operation,
		labelStatusCode: statusCode,
		labelErrorCode:  errorCode,
	}).Inc()
	m.instruments.apiRequestDurationSecond.With(map[string]string{
		labelService:   service,
		labelOperation: operation,
	}).Observe(duration.Seconds())

	return out, metadata, err
}

func errorCodeFromErr(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return "internal"
}
