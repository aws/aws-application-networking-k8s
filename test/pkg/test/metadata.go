package test

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/imdario/mergo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	sequentialNumber     = 0
	randomizer           = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint
	sequentialNumberLock = new(sync.Mutex)
	DiscoveryLabel       = "testing.kubernetes.io-" + randomdata.Alphanumeric(8) // Each time test suite run have a different label for k8s resource
)

func New[T client.Object](t T, mergeFrom ...T) T {
	if t.GetName() == "" {
		t.SetName(RandomName())
	}
	if t.GetNamespace() == "" {
		t.SetNamespace("default")
	}
	t.SetLabels(map[string]string{DiscoveryLabel: "true"})
	return MustMerge(t, mergeFrom...)
}

func MustMerge[T interface{}](dest T, srcs ...T) T {
	for _, src := range srcs {
		if err := mergo.Merge(&dest, src, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
			panic(fmt.Sprintf("Failed to merge object: %s", err))
		}
	}
	return dest
}

func RandomName() string {
	sequentialNumberLock.Lock()
	defer sequentialNumberLock.Unlock()
	sequentialNumber++
	return strings.ToLower(fmt.Sprintf("%s-%d-%s", randomdata.SillyName()[:5], sequentialNumber, randomdata.Alphanumeric(10)))
}
