package lattice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

// Make-before-break target replacement (ENABLE_MAKE_BEFORE_BREAK_TARGETS).
//
// When a Service selector changes, every endpoint of a target group can be
// replaced in one reconcile (for example a blue/green promotion). Removing the
// old targets before the new ones pass health checks leaves the target group
// with no healthy target. With the flag on, Update registers the new targets
// first and keeps a departing target registered while:
//   - some desired target is not yet ready (see notReadyReplacements),
//   - the departing target's pod still exists, is Ready and is not being deleted,
//   - it has been held for less than MAKE_BEFORE_BREAK_TARGETS_TIMEOUT.
//
// The targets synthesizer requeues while any target is held (hasHeldTargets).

type holdKey struct {
	tgId string
	ip   string
	port int64
}

// targetHolds remembers when each held target was first kept, so the timeout
// survives across reconciles. It is process-wide because the service export
// deployer builds a new TargetsManager per Deploy.
type targetHolds struct {
	mu    sync.Mutex
	now   func() time.Time
	since map[holdKey]time.Time
}

func newTargetHolds(now func() time.Time) *targetHolds {
	return &targetHolds{now: now, since: map[holdKey]time.Time{}}
}

var defaultTargetHolds = newTargetHolds(time.Now)

// start returns when key was first held, recording now if it is new.
func (h *targetHolds) start(key holdKey, now time.Time) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t, ok := h.since[key]; ok {
		return t
	}
	h.since[key] = now
	return now
}

// forget drops every hold of tgId, for a target group that is being deleted.
func (h *targetHolds) forget(tgId string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k := range h.since {
		if k.tgId == tgId {
			delete(h.since, k)
		}
	}
}

// retain drops every entry of tgId whose key is not in keep.
func (h *targetHolds) retain(tgId string, keep map[holdKey]struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k := range h.since {
		if k.tgId != tgId {
			continue
		}
		if _, ok := keep[k]; !ok {
			delete(h.since, k)
		}
	}
}

func (h *targetHolds) len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.since)
}

// notReadyReplacements describes each desired target that cannot take traffic
// yet, going by the VPC Lattice status read before this reconcile registered
// anything. A target missing from that read (or DRAINING, so about to be
// registered again) is not ready. UNAVAILABLE (health checks off) and UNUSED
// (target group not attached, no traffic) do not wait on a health check, so
// they count as ready. An empty result means every replacement is ready.
func notReadyReplacements(modelTargets []model.Target, latticeTargets []types.TargetSummary) []string {
	status := make(map[model.Target]types.TargetStatus, len(latticeTargets))
	for _, t := range latticeTargets {
		status[model.Target{TargetIP: aws.ToString(t.Id), Port: int64(aws.ToInt32(t.Port))}] = t.Status
	}
	var notReady []string
	for _, t := range modelTargets {
		s, registered := status[model.Target{TargetIP: t.TargetIP, Port: t.Port}]
		if registered && (s == types.TargetStatusHealthy || s == types.TargetStatusUnavailable || s == types.TargetStatusUnused) {
			continue
		}
		if !registered {
			s = "NOT_REGISTERED_YET"
		}
		notReady = append(notReady, fmt.Sprintf("%s:%d (%s)", t.TargetIP, t.Port, s))
	}
	return notReady
}

// maxLoggedTargets caps how many target names one hold log line lists.
const maxLoggedTargets = 5

func joinCapped(items []string) string {
	if len(items) <= maxLoggedTargets {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:maxLoggedTargets], ", "), len(items)-maxLoggedTargets)
}

// hasHeldTargets reports whether the target group still has a registered,
// non-draining target outside the model. With the flag on that is a target
// being held, so the caller must reconcile again.
func hasHeldTargets(modelTargets *model.Targets, latticeTargets []types.TargetSummary) bool {
	return len(findStaleTargets(modelTargets, latticeTargets)) > 0
}

// livePodIPs returns the IPs owned by exactly one Ready, non-terminating,
// non-hostNetwork pod in the namespaces of the model's pod targets.
func (s *defaultTargetsManager) livePodIPs(ctx context.Context, modelTargets []model.Target) (map[string]bool, error) {
	namespaces := utils.NewSet[string]()
	for _, t := range modelTargets {
		if t.TargetRef.Namespace != "" {
			namespaces.Put(t.TargetRef.Namespace)
		}
	}
	owners := map[string]int{}
	live := map[string]bool{}
	for _, ns := range namespaces.Items() {
		pods := &corev1.PodList{}
		if err := s.client.List(ctx, pods, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			ips := utils.NewSet[string]()
			if pod.Status.PodIP != "" {
				ips.Put(pod.Status.PodIP)
			}
			for _, ip := range pod.Status.PodIPs {
				ips.Put(ip.IP)
			}
			ok := !pod.Spec.HostNetwork && pod.DeletionTimestamp == nil && podReady(pod)
			for _, ip := range ips.Items() {
				owners[ip]++
				if ok {
					live[ip] = true
				}
			}
		}
	}
	for ip, n := range owners {
		if n != 1 {
			delete(live, ip)
		}
	}
	return live, nil
}

func podReady(pod *corev1.Pod) bool {
	cond := utils.FindPodStatusCondition(pod.Status.Conditions, corev1.PodReady)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

func (s *defaultTargetsManager) updateMakeBeforeBreak(
	ctx context.Context,
	modelTargets *model.Targets,
	modelTg *model.TargetGroup,
	latticeTargets []types.TargetSummary,
	staleTargets []model.Target,
) error {
	tgId := modelTg.Status.Id
	regErr := s.registerTargets(ctx, modelTg, s.findNewTargets(modelTargets, latticeTargets))

	notReady := notReadyReplacements(modelTargets.Spec.TargetList, latticeTargets)
	ready := len(notReady) == 0
	var live map[string]bool
	if !ready && len(staleTargets) > 0 {
		var err error
		if live, err = s.livePodIPs(ctx, modelTargets.Spec.TargetList); err != nil {
			// Cannot tell whether old pods are alive: fall back to deregistering.
			s.log.Warnf(ctx, "Cannot list pods to hold targets of target group %s, deregistering: %s", tgId, err)
			live = nil
		}
	}

	now := s.holds.now()
	keep := map[holdKey]struct{}{}
	var drop []model.Target
	held := 0
	for _, t := range staleTargets {
		key := holdKey{tgId: tgId, ip: t.TargetIP, port: t.Port}
		if !ready && live[t.TargetIP] {
			since := s.holds.start(key, now)
			if now.Sub(since) < config.MakeBeforeBreakTargetsTimeout {
				keep[key] = struct{}{}
				held++
				continue
			}
			s.log.Infof(ctx, "Target %s:%d of target group %s held for %s, past the timeout; deregistering",
				t.TargetIP, t.Port, tgId, now.Sub(since))
		}
		drop = append(drop, t)
	}
	if held > 0 {
		s.log.Infof(ctx, "Holding %d departing targets in target group %s until replacements are healthy, not ready: %s",
			held, tgId, joinCapped(notReady))
	}

	deregErr := s.deregisterTargets(ctx, modelTg, drop)
	if deregErr != nil {
		// Keep the timers of targets we failed to remove, so a retry does not restart them.
		for _, t := range drop {
			keep[holdKey{tgId: tgId, ip: t.TargetIP, port: t.Port}] = struct{}{}
		}
	}
	s.holds.retain(tgId, keep)
	return errors.Join(regErr, deregErr)
}
