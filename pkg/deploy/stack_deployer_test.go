package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
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
	ruleQuotaErr := fmt.Errorf("error during rule synthesis %w", &lattice.RuleQuotaExceededError{Err: errors.New("quota")})
	pending := errors.New("targets pending")

	tests := []struct {
		name         string
		ruleQuotaErr error
		postErr      error
		deleteErr    error
		wantRuleErr  bool
		wantErr      error
		wantDeletes  int
	}{
		{name: "no errors", wantDeletes: 1},
		{name: "step error without quota error is returned and stops", postErr: pending, wantErr: pending},
		{name: "quota error alone is returned after all steps", ruleQuotaErr: ruleQuotaErr, wantRuleErr: true, wantDeletes: 1},
		{
			// a pending readiness gate returns a 10 s retry error; it must not replace the quota error
			name: "quota error wins over later step errors", ruleQuotaErr: ruleQuotaErr,
			postErr: pending, deleteErr: errors.New("tg in use"), wantRuleErr: true, wantDeletes: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deletes := 0
			d := &latticeServiceStackDeployer{log: gwlog.FallbackLogger}
			err := d.runAfterRules(ctx, tt.ruleQuotaErr,
				afterRulesStep{"post", func(context.Context) error { return tt.postErr }},
				afterRulesStep{"delete", func(context.Context) error { deletes++; return tt.deleteErr }},
			)
			switch {
			case tt.wantRuleErr:
				assert.Same(t, tt.ruleQuotaErr, err)
				_, ok := lattice.AsRuleQuotaExceeded(err)
				assert.True(t, ok)
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			default:
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantDeletes, deletes)
		})
	}
}
