package lattice

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// The scan picks a candidate whose ServiceExport is gone. By the time GC holds the
// lock the candidate may have changed, so GC must re-read it and decide again.
func Test_SynthesizeUnusedDeleteWithLock_ReChecksUnderLock(t *testing.T) {
	setGcTestConfig(t)
	notFound := &apierrors.StatusError{ErrStatus: metav1.Status{Code: 404, Reason: metav1.StatusReasonNotFound}}

	tests := []struct {
		name        string
		current     func(scanned types.TargetGroupSummary) *types.TargetGroupSummary
		getErr      error
		k8sGets     int // scan, plus the re-check if it gets that far
		wantDelete  bool
		wantResults int
		wantErr     string
	}{
		{
			name:        "still unused is deleted",
			current:     func(s types.TargetGroupSummary) *types.TargetGroupSummary { return &s },
			k8sGets:     2,
			wantDelete:  true,
			wantResults: 1,
		},
		{
			name: "attached to a service since the scan is kept",
			current: func(s types.TargetGroupSummary) *types.TargetGroupSummary {
				s.ServiceArns = []string{"svc-arn"}
				return &s
			},
			k8sGets: 1,
		},
		{
			name:    "already deleted is skipped",
			current: func(types.TargetGroupSummary) *types.TargetGroupSummary { return nil },
			k8sGets: 1,
		},
		{
			name:        "re-read error is reported and not deleted",
			getErr:      errors.New("throttled"),
			k8sGets:     1,
			wantResults: 1,
			wantErr:     "throttled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()
			mockTGManager := NewMockTargetGroupManager(c)
			mockClient := mock_client.NewMockClient(c)
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound).Times(tt.k8sGets)

			var rw sync.RWMutex
			scanned := svcExportTg("arn-a", "a")
			mockTGManager.EXPECT().List(ctx).DoAndReturn(
				func(context.Context) ([]tgListOutput, error) {
					assert.True(t, rw.TryLock(), "scan must not hold the GC lock")
					rw.Unlock()
					return []tgListOutput{scanned}, nil
				})
			var current *types.TargetGroupSummary
			if tt.current != nil {
				current = tt.current(scanned.tgSummary)
			}
			mockTGManager.EXPECT().Get(ctx, "arn-a").DoAndReturn(
				func(context.Context, string) (*types.TargetGroupSummary, error) {
					assert.False(t, rw.TryRLock(), "re-read must hold the GC lock")
					return current, tt.getErr
				})
			if tt.wantDelete {
				mockTGManager.EXPECT().Delete(ctx, gomock.Any()).DoAndReturn(
					func(_ context.Context, tg *model.TargetGroup) error {
						assert.False(t, rw.TryRLock(), "delete must hold the GC lock")
						assert.Equal(t, "arn-a", tg.Status.Arn)
						return nil
					})
			}

			synth := NewTargetGroupSynthesizer(gwlog.FallbackLogger, nil, mockClient, mockTGManager, nil, nil, nil)
			results, err := synth.SynthesizeUnusedDeleteWithLock(ctx, &rw)
			require.NoError(t, err)
			require.Len(t, results, tt.wantResults)
			if tt.wantErr != "" {
				assert.ErrorContains(t, results[0].Err, tt.wantErr)
			}
			assert.True(t, rw.TryLock(), "lock must be released")
		})
	}
}

// A deploy that runs between the unlocked scan and the delete must win. The deploy
// runs, holding the read lock as stack deployers do, right before GC takes the
// write lock, which is the widest the window can be.
func Test_SynthesizeUnusedDeleteWithLock_DeployBetweenScanAndDelete(t *testing.T) {
	setGcTestConfig(t)
	cases := map[string]func(w *gcWorld){
		// the ServiceExport is recreated; its deploy finds the existing TG
		"service export recreated and TG reused": func(w *gcWorld) {
			w.createExport(t, "a")
			w.deploy("a")
		},
		// a route rule now forwards to the TG
		"TG attached to a service": func(w *gcWorld) {
			w.attach("arn-a")
		},
	}
	for name, deployFn := range cases {
		t.Run(name, func(t *testing.T) {
			w := newGcWorld(t)
			w.addTg("arn-a", "a") // the TG exists but its ServiceExport does not: a GC candidate
			var once sync.Once
			lock := lockFunc{
				lock: func() {
					once.Do(func() {
						w.rw.RLock()
						defer w.rw.RUnlock()
						deployFn(w)
					})
					w.rw.Lock()
				},
				unlock: w.rw.Unlock,
			}
			results, err := w.synth().SynthesizeUnusedDeleteWithLock(context.Background(), lock)
			require.NoError(t, err)
			assert.Empty(t, results, "no delete should be attempted")
			assert.Empty(t, w.wrongful(), "GC deleted a TG a deploy was using")
			assert.True(t, w.exists("arn-a"))
		})
	}
}

// Deploys and GC run concurrently. ServiceExports are created and deleted without
// any lock; deploys hold the read lock while they find or create the TG and mark it
// in use. GC must never delete a TG that is in use. Run with -race.
func Test_SynthesizeUnusedDeleteWithLock_ConcurrentDeploys(t *testing.T) {
	setGcTestConfig(t)
	w := newGcWorld(t)
	w.deleteDelay = 200 * time.Microsecond

	const exports = 8
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var deploys atomic.Int64
	for i := 0; i < exports; i++ {
		name := fmt.Sprintf("svc-%d", i)
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(seed))
			for ctx.Err() == nil {
				w.createExport(t, name)
				w.rw.RLock()
				w.deploy(name)
				w.rw.RUnlock()
				deploys.Add(1)
				time.Sleep(time.Duration(rnd.Intn(300)) * time.Microsecond)
				w.release(name)
				w.deleteExport(t, name)
				time.Sleep(time.Duration(rnd.Intn(300)) * time.Microsecond)
			}
		}(int64(i))
	}

	var cycles, deleted int
	for ctx.Err() == nil {
		results, err := w.synth().SynthesizeUnusedDeleteWithLock(context.Background(), &w.rw)
		require.NoError(t, err)
		for _, r := range results {
			if r.Err == nil {
				deleted++
			}
		}
		cycles++
	}
	wg.Wait()

	t.Logf("gc cycles=%d deletes=%d deploys=%d", cycles, deleted, deploys.Load())
	assert.Positive(t, deleted, "GC should delete TGs of deleted exports")
	assert.Empty(t, w.wrongful(), "GC deleted TGs that deploys were using")
}

func setGcTestConfig(t *testing.T) {
	prevVpc, prevCluster := config.VpcID, config.ClusterName
	t.Cleanup(func() { config.VpcID, config.ClusterName = prevVpc, prevCluster })
	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"
}

func svcExportTg(arn, svc string) tgListOutput {
	return tgListOutput{
		tgSummary: types.TargetGroupSummary{
			Arn: aws.String(arn), Id: aws.String("id-" + arn), Name: aws.String("name-" + arn),
			VpcIdentifier: aws.String("vpc-id"), Port: aws.Int32(80), Protocol: types.TargetGroupProtocolHttp,
			IpAddressType: types.IpAddressTypeIpv4,
		},
		tags: model.TagsFromTGTagFields(model.TargetGroupTagFields{
			K8SClusterName: "cluster-name", K8SSourceType: model.SourceTypeSvcExport,
			K8SServiceName: svc, K8SServiceNamespace: "ns", K8SProtocolVersion: "HTTP1",
		}),
	}
}

type lockFunc struct{ lock, unlock func() }

func (l lockFunc) Lock()   { l.lock() }
func (l lockFunc) Unlock() { l.unlock() }

// gcWorld is an in-memory VPC Lattice (target groups only) and Kubernetes API for GC
// concurrency tests. It records GC deletes of TGs that a deploy was using.
type gcWorld struct {
	TargetGroupManager // only the methods below are used

	rw          sync.RWMutex // the GC lock
	k8s         client.Client
	deleteDelay time.Duration

	mu      sync.Mutex
	tgs     map[string]*worldTg // by ARN
	seq     int
	badDels []string
}

type worldTg struct {
	out   tgListOutput
	inUse bool // a deploy found or created it, and its ServiceExport has not been deleted since
}

func newGcWorld(t *testing.T) *gcWorld {
	scheme := runtime.NewScheme()
	require.NoError(t, anv1alpha1.Install(scheme))
	return &gcWorld{
		k8s: fake.NewClientBuilder().WithScheme(scheme).Build(),
		tgs: map[string]*worldTg{},
	}
}

func (w *gcWorld) synth() *TargetGroupSynthesizer {
	return NewTargetGroupSynthesizer(gwlog.FallbackLogger, nil, w.k8s, w, upToDateSvcExportBuilder{}, nil, nil)
}

func (w *gcWorld) addTg(arn, svc string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tgs[arn] = &worldTg{out: svcExportTg(arn, svc)}
}

func (w *gcWorld) exists(arn string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tgs[arn] != nil
}

func (w *gcWorld) wrongful() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.badDels...)
}

func (w *gcWorld) createExport(t *testing.T, name string) {
	se := &anv1alpha1.ServiceExport{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
	assert.NoError(t, w.k8s.Create(context.Background(), se))
}

func (w *gcWorld) deleteExport(t *testing.T, name string) {
	se := &anv1alpha1.ServiceExport{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
	assert.NoError(t, w.k8s.Delete(context.Background(), se))
}

// deploy is the TG part of a ServiceExport deploy: find the TG for svc or create it,
// then use it. The caller holds the GC read lock.
func (w *gcWorld) deploy(svc string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, tg := range w.tgs {
		if model.TGTagFieldsFromTags(tg.out.tags).K8SServiceName == svc {
			tg.inUse = true
			return
		}
	}
	w.seq++
	arn := fmt.Sprintf("arn-%s-%d", svc, w.seq)
	w.tgs[arn] = &worldTg{out: svcExportTg(arn, svc), inUse: true}
}

// release marks the TG of svc unused, just before its ServiceExport is deleted.
func (w *gcWorld) release(svc string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, tg := range w.tgs {
		if model.TGTagFieldsFromTags(tg.out.tags).K8SServiceName == svc {
			tg.inUse = false
		}
	}
}

func (w *gcWorld) attach(arn string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tgs[arn].out.tgSummary.ServiceArns = []string{"svc-arn"}
	w.tgs[arn].inUse = true
}

func (w *gcWorld) List(context.Context) ([]tgListOutput, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []tgListOutput
	for _, tg := range w.tgs {
		out = append(out, tg.out)
	}
	return out, nil
}

func (w *gcWorld) Get(_ context.Context, arn string) (*types.TargetGroupSummary, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	tg := w.tgs[arn]
	if tg == nil {
		return nil, nil
	}
	s := tg.out.tgSummary
	return &s, nil
}

func (w *gcWorld) Delete(_ context.Context, modelTg *model.TargetGroup) error {
	time.Sleep(w.deleteDelay) // ListTargets, DeregisterTargets, DeleteTargetGroup
	w.mu.Lock()
	defer w.mu.Unlock()
	tg := w.tgs[modelTg.Status.Arn]
	if tg == nil {
		return nil
	}
	if tg.inUse {
		w.badDels = append(w.badDels, modelTg.Status.Arn)
	}
	delete(w.tgs, modelTg.Status.Arn)
	return nil
}

// upToDateSvcExportBuilder builds the model that every svcExportTg matches.
type upToDateSvcExportBuilder struct {
	gateway.SvcExportTargetGroupModelBuilder
}

func (upToDateSvcExportBuilder) BuildTargetGroup(context.Context, *anv1alpha1.ServiceExport) (*model.TargetGroup, error) {
	return &model.TargetGroup{Spec: model.TargetGroupSpec{
		Port: 80, Protocol: "HTTP", ProtocolVersion: "HTTP1", IpAddressType: "IPV4",
	}}, nil
}
