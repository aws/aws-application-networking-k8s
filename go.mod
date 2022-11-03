module github.com/aws/aws-application-networking-k8s

go 1.16

require (
	github.com/aws/aws-sdk-go v1.42.18
	github.com/go-logr/logr v0.4.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/mock v1.5.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/gateway-api v0.4.0
	sigs.k8s.io/mcs-api v0.1.0
)

replace github.com/aws/aws-sdk-go => ./scripts/aws_sdk_model_override/aws-sdk-go
