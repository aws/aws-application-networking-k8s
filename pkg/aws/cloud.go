package aws

import (
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
)

type Cloud interface {
	Lattice() services.Lattice
	EKS() services.EKS
	SetVpcID(vpcID string)
	GetAccountID() string
	GetServiceNetworkName() string
	GetVpcID() string
	UseLongTGName() bool
}

// NewCloud constructs new Cloud implementation.
func NewCloud(log gwlog.Logger, region string, accountID string, svcNetwork string, vpcID string, useTGLongName bool) (Cloud, error) {
	// TODO: need to pass cfg CloudConfig later
	sess, _ := session.NewSession()

	sess.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {
			log.Debugw("error",
				"error", r.Error.Error(),
				"serviceName", r.ClientInfo.ServiceName,
				"operation", r.Operation.Name,
				"params", r.Params,
			)
		} else {
			log.Debugw("response",
				"serviceName", r.ClientInfo.ServiceName,
				"operation", r.Operation.Name,
				"params", r.Params,
			)
		}
	})

	return &defaultCloud{
		// TODO: service
		vpcLatticeSess: services.NewDefaultLattice(sess, region),
		eksSess:        services.NewDefaultEKS(sess, region),
		accountID:      accountID,
		svcNetwork:     svcNetwork,
		vpcID:          vpcID,
		useTGLongName:  useTGLongName,
	}, nil
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	vpcLatticeSess services.Lattice
	eksSess        services.EKS
	accountID      string
	svcNetwork     string
	vpcID          string
	useTGLongName  bool
}

func (d *defaultCloud) Lattice() services.Lattice {
	return d.vpcLatticeSess
}

func (d *defaultCloud) EKS() services.EKS {
	return d.eksSess
}

func (d *defaultCloud) GetEKSClusterVPC(name string) string {
	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}

	result, err := d.eksSess.DescribeCluster(input)

	if err != nil {
		fmt.Printf("Erron eks DescridbeCluster %v\n", err)
		return ""
	}
	return (result.String())
}

func (d *defaultCloud) SetVpcID(vpcID string) {
	d.vpcID = vpcID
}

func (d *defaultCloud) GetAccountID() string {
	return d.accountID
}

func (d *defaultCloud) GetServiceNetworkName() string {
	return d.svcNetwork
}

func (d *defaultCloud) GetVpcID() string {
	return d.vpcID
}

func (d *defaultCloud) UseLongTGName() bool {
	return d.useTGLongName
}
