## Note: mocks for interfaces from this project should be along with the original package.
##       mocks for interfaces from 3rd-party project should be put inside ./mocks folder.
## mockgen version v1.5.0
mockgen -package=mock_client -destination=./mocks/controller-runtime/client/client_mocks.go sigs.k8s.io/controller-runtime/pkg/client Client
mockgen -package=mock_client -destination=./mocks/controller-runtime/client/record_mock.go k8s.io/client-go/tools/record EventRecorder
mockgen -package=services -destination=./pkg/aws/services/vpclattice_mocks.go -source=./pkg/aws/services/vpclattice.go
mockgen -package=aws -destination=./pkg/aws/cloud_mocks.go -source=./pkg/aws/cloud.go
mockgen -package=lattice -destination=./pkg/deploy/lattice/service_network_manager_mock.go -source=./pkg/deploy/lattice/service_network_manager.go
mockgen -package=lattice -destination=./pkg/deploy/lattice/target_group_manager_mock.go -source=./pkg/deploy/lattice/target_group_manager.go
mockgen -package=lattice -destination=./pkg/deploy/lattice/targets_manager_mock.go -source=./pkg/deploy/lattice/targets_manager.go
mockgen -package=lattice -destination=./pkg/deploy/lattice/service_manager_mock.go -source=./pkg/deploy/lattice/service_manager.go
mockgen -package=lattice -destination=./pkg/deploy/lattice/listener_manager_mock.go -source=./pkg/deploy/lattice/listener_manager.go
mockgen -package=lattice -destination=./pkg/deploy/lattice/rule_manager_mock.go -source=./pkg/deploy/lattice/rule_manager.go
mockgen -package=k8s -destination=./pkg/k8s/finalizer_mock.go -source=./pkg/k8s/finalizer.go FinalizerManager
mockgen -package=gateway -destination=./pkg/gateway/model_build_lattice_service_mock.go -source=./pkg/gateway/model_build_lattice_service.go
mockgen -package=gateway -destination=./pkg/gateway/model_build_targetgroup_mock.go -source=./pkg/gateway/model_build_targetgroup.go
mockgen -package=externaldns -destination=./pkg/deploy/externaldns/dnsendpoint_manager_mock.go -source=./pkg/deploy/externaldns/dnsendpoint_manager.go
# need some manual update to remote core for stack_mock.go
mockgen -package=core -destination=./pkg/model/core/stack_mock.go -source=./pkg/model/core/stack.go
