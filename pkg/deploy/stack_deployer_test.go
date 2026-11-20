package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/stretchr/testify/assert"
)

func TestTgGc(t *testing.T) {

	type test struct {
		name string
		gcFn TgGcCycleFn
	}

	tests := []test{
		{
			name: "empty cycle",
			gcFn: func(context.Context) (TgGcResult, error) { return TgGcResult{}, nil },
		},
		{
			name: "cycle with results",
			gcFn: func(context.Context) (TgGcResult, error) {
				return TgGcResult{
					att:      10,
					succ:     10,
					duration: time.Second,
				}, nil
			},
		},
		{
			name: "cycle panic",
			gcFn: func(context.Context) (TgGcResult, error) { panic("") },
		},
		{
			name: "cycle error",
			gcFn: func(context.Context) (TgGcResult, error) { return TgGcResult{}, errors.New("") },
		},
	}

	nCycles := 10 // run each test at least 10 cycles

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			n := 0
			f := func(ctx context.Context) (TgGcResult, error) {
				defer func() {
					n += 1
					if n >= nCycles {
						cancel()
					}
				}()
				return tt.gcFn(ctx)
			}
			ivl := time.Millisecond * 10
			tgGc := &TgGc{
				log:     gwlog.FallbackLogger,
				ctx:     ctx,
				ivl:     ivl,
				cycleFn: f,
			}
			tgGc.start()
			time.Sleep(ivl * (time.Duration(nCycles) + 2)) // sleep enough cycles to terminate
			assert.Equal(t, nCycles, n, fmt.Sprintf("should run only %d cycles", nCycles))
			assert.True(t, tgGc.isDone.Load(), "gc must terminate")
		})
	}
}

func TestRunAfterRules(t *testing.T) {
	ctx := context.TODO()
	pending := fmt.Errorf("%w: target status still in pending", lattice_runtime.NewRetryError())
	newQuotaErr := func() error {
		return fmt.Errorf("error during rule synthesis %w", &lattice.RuleQuotaExceededError{Err: errors.New("quota")})
	}

	tests := []struct {
		name              string
		quota             bool
		postErr           error
		deleteErr         error
		wantErr           error
		wantDeletes       int
		wantTargetPending bool
	}{
		{name: "no errors", wantDeletes: 1},
		{name: "step error without quota error is returned and stops", postErr: pending, wantErr: pending},
		{name: "quota error alone is returned after all steps", quota: true, wantDeletes: 1},
		{
			// a pending readiness gate must not replace the quota error, but is recorded on it
			name: "quota error wins over later step errors", quota: true,
			postErr: pending, deleteErr: errors.New("tg in use"), wantDeletes: 1, wantTargetPending: true,
		},
		{name: "a non-retry readiness step error is not pending targets", quota: true, postErr: errors.New("list failed"), wantDeletes: 1},
		{name: "a retry error from tg delete is not pending targets", quota: true, deleteErr: lattice_runtime.NewRetryError(), wantDeletes: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ruleQuotaErr error
			if tt.quota {
				ruleQuotaErr = newQuotaErr()
			}
			deletes := 0
			d := &latticeServiceStackDeployer{log: gwlog.FallbackLogger}
			err := d.runAfterRules(ctx, ruleQuotaErr,
				afterRulesStep{name: "post", run: func(context.Context) error { return tt.postErr }, readinessGates: true},
				afterRulesStep{name: "delete", run: func(context.Context) error { deletes++; return tt.deleteErr }},
			)
			switch {
			case tt.quota:
				assert.Same(t, ruleQuotaErr, err)
				ruleErr, ok := lattice.AsRuleQuotaExceeded(err)
				if assert.True(t, ok) {
					assert.Equal(t, tt.wantTargetPending, ruleErr.TargetsPending)
				}
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			default:
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantDeletes, deletes)
		})
	}
}
