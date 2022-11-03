module github.com/aws/aws-application-networking-k8s

go 1.16

require (
	github.com/aws/aws-sdk-go v1.44.114
	github.com/aws/karpenter-core v0.0.2-0.20221103224619-449d90b796ed
	github.com/go-logr/logr v1.2.3
	github.com/golang/glog v1.0.0
	github.com/golang/mock v1.6.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.23.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.0
	k8s.io/api v0.25.2
	k8s.io/apimachinery v0.25.2
	k8s.io/client-go v0.25.2
	sigs.k8s.io/controller-runtime v0.13.0
	sigs.k8s.io/gateway-api v0.4.0
	sigs.k8s.io/mcs-api v0.1.0
)

replace github.com/aws/aws-sdk-go => ./scripts/aws_sdk_model_override/aws-sdk-go
