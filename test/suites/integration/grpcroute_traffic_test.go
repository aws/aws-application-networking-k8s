package integration

import (
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"strconv"
	"time"
)

var _ = Describe("GRPCRoute traffic test", func() {

	var (
		grpcBinDeployment        *appsv1.Deployment
		grpcBinService           *v1.Service
		grpcHelloWorldDeployment *appsv1.Deployment
		grpcHelloWorldService    *v1.Service
	)

	BeforeEach(func() {
		grpcBinDeployment, grpcBinService = testFramework.NewGrpcBin(test.GrpcAppOptions{AppName: "my-grpcbin-1", Namespace: k8snamespace})
		grpcHelloWorldDeployment, grpcHelloWorldService = testFramework.NewGrpcHelloWorld(test.GrpcAppOptions{AppName: "my-grpc-hello-world-1", Namespace: k8snamespace})
		testFramework.ExpectCreated(ctx, grpcBinDeployment, grpcBinService, grpcHelloWorldDeployment, grpcHelloWorldService)
		time.Sleep(10 * time.Second)

	})

	It("Test GRPC traffic by k8s network only and don't use vpc lattice, "+
		"only a demo for how to use grpc helper functions, this test case will be removed later", func() {

		grpcurlCmdOptions := test.RunGrpcurlCmdOptions{
			GrpcServerHostName: grpcBinService.Name + "." + grpcBinService.Namespace + ".svc.cluster.local",
			GrpcServerPort:     "19000",
			Service:            " grpcbin.GRPCBin",
			Method:             "DummyUnary",
			ReqParamsJsonString: `{
								  "f_string": "myTestString",
								  "f_int32": 42,
								  "f_bool": true,
								  "f_bytes": "SGVsbG8gV29ybGQ="
								}`,
			UseTLS: false,
		}
		stdoutStr, stderrStr, err := testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
		Expect(err).To(BeNil())
		Expect(stderrStr).To(BeEmpty())
		Expect(stdoutStr).To(ContainSubstring(`"fString": "myTestString"`))
		Expect(stdoutStr).To(ContainSubstring(`"fInt32": 42`))

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  grpcBinService.Name + "." + grpcBinService.Namespace + ".svc.cluster.local",
			GrpcServerPort:      "19001",
			Service:             " addsvc.Add",
			Method:              "Sum",
			ReqParamsJsonString: `{"a": 5, "b": 6}`,
			UseTLS:              true,
		}
		stdoutStr, stderrStr, err = testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
		Expect(err).To(BeNil())
		Expect(stderrStr).To(BeEmpty())
		Expect(stdoutStr).To(ContainSubstring("\"v\": " + strconv.Itoa(5+6)))

		grpcurlCmdOptions = test.RunGrpcurlCmdOptions{
			GrpcServerHostName:  grpcHelloWorldService.Name + "." + grpcHelloWorldService.Namespace + ".svc.cluster.local",
			GrpcServerPort:      "10051",
			Service:             "helloworld.Greeter",
			Method:              "SayHello",
			ReqParamsJsonString: `{"name": "myTestName"}`,
			UseTLS:              false,
		}
		stdoutStr, stderrStr, err = testFramework.RunGrpcurlCmd(grpcurlCmdOptions)
		Expect(err).To(BeNil())
		Expect(stderrStr).To(BeEmpty())
		Expect(stdoutStr).To(ContainSubstring("Hello myTestName"))

	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			grpcBinService,
			grpcBinDeployment,
			grpcHelloWorldService,
			grpcHelloWorldDeployment,
		)
	})
})
