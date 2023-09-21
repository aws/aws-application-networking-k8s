package gateway

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	ResourceIDServiceNetwork        = "ServiceNetwork"
	LatticeVPCAssociationAnnotation = "application-networking.k8s.aws/lattice-vpc-association"
	ModelBuiltError                 = "Failed to build model"
)

// ModelBuilder builds the model stack for the mesh resource.
type ServiceNetworkModelBuilder interface {
	// Build model stack for service
	Build(ctx context.Context, gw *gateway_api.Gateway) (core.Stack, *latticemodel.ServiceNetwork, error)
}

type serviceNetworkModelBuilder struct {
	client      client.Client
	defaultTags map[string]string
}

func NewServiceNetworkModelBuilder(client client.Client) *serviceNetworkModelBuilder {
	return &serviceNetworkModelBuilder{client: client}
}
func (b *serviceNetworkModelBuilder) Build(ctx context.Context, gw *gateway_api.Gateway) (core.Stack, *latticemodel.ServiceNetwork, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(gw)))
	vpcAssociationPolicy, err := GetAttachedPolicy(ctx, b.client, k8s.NamespacedName(gw), &v1alpha1.VpcAssociationPolicy{})
	if err != nil {
		return nil, nil, err
	}
	task := &serviceNetworkModelBuildTask{
		gateway:              gw,
		vpcAssociationPolicy: vpcAssociationPolicy,
		stack:                stack,
	}

	if err := task.run(ctx); err != nil {
		return nil, nil, corev1.ErrIntOverflowGenerated
	}

	return task.stack, task.serviceNetwork, nil
}

func (t *serviceNetworkModelBuildTask) run(ctx context.Context) error {

	err := t.buildModel(ctx)
	return err
}

func (t *serviceNetworkModelBuildTask) buildModel(ctx context.Context) error {
	err := t.buildServiceNetwork(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (t *serviceNetworkModelBuildTask) buildServiceNetwork(ctx context.Context) error {
	spec := latticemodel.ServiceNetworkSpec{
		Name:      t.gateway.Name,
		Namespace: t.gateway.Namespace,
		Account:   config.AccountID,
	}

	// default associateToVPC is true
	associateToVPC := true
	if value, exist := t.gateway.Annotations[LatticeVPCAssociationAnnotation]; exist {
		associateToVPC = value == "true"
	}

	if t.vpcAssociationPolicy != nil {
		if t.vpcAssociationPolicy.Spec.AssociateWithVpc != nil {
			associateToVPC = *t.vpcAssociationPolicy.Spec.AssociateWithVpc
		}

		if t.vpcAssociationPolicy.Spec.SecurityGroupIds != nil {
			spec.SecurityGroupIds = securityGroupIdsToStringPointersSlice(t.vpcAssociationPolicy.Spec.SecurityGroupIds)
		}
	}
	spec.AssociateToVPC = associateToVPC
	defaultSN, err := config.GetClusterLocalGateway()

	if err == nil && defaultSN != t.gateway.Name {
		// there is a default gateway for local cluster, all other gateways are not associate to VPC
		spec.AssociateToVPC = false
	}

	if !t.gateway.DeletionTimestamp.IsZero() {
		spec.IsDeleted = true
	} else {
		spec.IsDeleted = false
	}

	t.serviceNetwork = latticemodel.NewServiceNetwork(t.stack, ResourceIDServiceNetwork, spec)

	return nil
}

type serviceNetworkModelBuildTask struct {
	gateway              *gateway_api.Gateway
	vpcAssociationPolicy *v1alpha1.VpcAssociationPolicy
	serviceNetwork       *latticemodel.ServiceNetwork

	stack core.Stack
}

func securityGroupIdsToStringPointersSlice(sgIds []v1alpha1.SecurityGroupId) []*string {
	var ret []*string
	for _, sgId := range sgIds {
		sgIdStr := string(sgId)
		ret = append(ret, &sgIdStr)
	}
	return ret
}
