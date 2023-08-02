package scripts

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("delete_all_resource_with_discovery_label_test", Ordered, func() {

	It("should delete all resources with discovery label", func() {
		testFramework.DeleteAllK8sResourceWithDiscoveryLabel(ctx)
		testFramework.ExpectToBeClean(ctx)
	})
})
