## Note: mocks for interfaces from this project should be along with the original package.
##       mocks for interfaces from 3rd-party project should be put inside ./mocks folder.
## mockgen version v1.5.0
~/go/bin/mockgen -package=mock_client -destination=./mocks/controller-runtime/client/client_mocks.go sigs.k8s.io/controller-runtime/pkg/client Client
~/go/bin/mockgen -package=services -destination=./pkg/aws/services/eks_mocks.go -source=./pkg/aws/services/eks.go
~/go/bin/mockgen -package=services -destination=./pkg/aws/services/mercury_mocks.go -source=./pkg/aws/services/mercury.go
~/go/bin/mockgen -package=aws -destination=./pkg/aws/cloud_mocks.go -source=./pkg/aws/cloud.go
~/go/bin/mockgen -package=lattice -destination=./pkg/deploy/lattice/service_network_manager_mock.go -source=./pkg/deploy/lattice/service_network_manager.go
~/go/bin/mockgen -package=lattice -destination=./pkg/deploy/lattice/target_group_manager_mock.go -source=./pkg/deploy/lattice/target_group_manager.go
~/go/bin/mockgen -package=lattice -destination=./pkg/deploy/lattice/targets_manager_mock.go -source=./pkg/deploy/lattice/targets_manager.go
~/go/bin/mockgen -package=lattice -destination=./pkg/deploy/lattice/service_manager_mock.go -source=./pkg/deploy/lattice/service_manager.go
~/go/bin/mockgen -package=lattice -destination=./pkg/deploy/lattice/listener_manager_mock.go -source=./pkg/deploy/lattice/listener_manager.go
~/go/bin/mockgen -package=lattice -destination=./pkg/deploy/lattice/rule_manager_mock.go -source=./pkg/deploy/lattice/rule_manager.go
# need some manual update to remote core for stack_mock.go
~/go/bin/mockgen -package=core -destination=./pkg/model/core/stack_mock.go -source=./pkg/model/core/stack.go
~/go/bin/mockgen -package=services -destination=./pkg/aws/services/mercury_service_api_mock.go -source=./scripts/aws_sdk_model_override/aws-sdk-go/service/mercury/mercuryiface/interface.go
