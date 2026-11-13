package deploy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
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
			tgGc := &TgGc{
				log:     gwlog.FallbackLogger,
				ivl:     time.Millisecond * 10,
				cycleFn: f,
			}
			done := make(chan error)
			go func() { done <- tgGc.Start(ctx) }()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("gc must terminate once ctx is done")
			}
			assert.Equal(t, nCycles, n, fmt.Sprintf("should run only %d cycles", nCycles))
		})
	}
}

func TestTgGcNeedsLeaderElection(t *testing.T) {
	var r manager.Runnable = &TgGc{}
	ler, ok := r.(manager.LeaderElectionRunnable)
	assert.True(t, ok)
	assert.True(t, ler.NeedLeaderElection(), "standby replicas must not run the TG GC")
}

func TestTgGcCycleExcludesDeploys(t *testing.T) {
	var inCycle atomic.Bool
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCycle := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCycle)
	tgGc := &TgGc{
		log: gwlog.FallbackLogger,
		cycleFn: func(context.Context) (TgGcResult, error) {
			inCycle.Store(true)
			<-release
			inCycle.Store(false)
			return TgGcResult{}, nil
		},
	}
	go tgGc.cycle(context.Background())
	assert.Eventually(t, inCycle.Load, time.Second, time.Millisecond)

	acquired := make(chan struct{})
	go func() {
		tgGcLock.RLock()
		defer tgGcLock.RUnlock()
		assert.False(t, inCycle.Load(), "a deploy must not run during a GC cycle")
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("deploy acquired the lock while a GC cycle was running")
	case <-time.After(50 * time.Millisecond):
	}
	releaseCycle()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("deploy must proceed once the GC cycle finishes")
	}
}
