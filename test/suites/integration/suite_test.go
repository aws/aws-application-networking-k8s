package integration

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

var testFramework *test.Framework
var ctx context.Context

func TestIntegration(t *testing.T) {
	ctx = test.NewContext(t)
	testFramework = test.NewFramework(ctx)
	RegisterFailHandler(Fail)
	//TODO: re-consider whether we really need ExpectToBeClean() in BeforeEach/AfterEach
	BeforeEach(func() { testFramework.ExpectToBeClean(ctx) })
	AfterEach(func() { testFramework.ExpectToBeClean(ctx) })
	RunSpecs(t, "Integration")
}
