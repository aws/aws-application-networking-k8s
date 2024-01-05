package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
