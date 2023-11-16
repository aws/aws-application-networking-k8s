package mock_client

//go:generate mockgen -package mock_client -destination client_mocks.go sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate mockgen -package mock_client -destination record_mock.go k8s.io/client-go/tools/record EventRecorder
