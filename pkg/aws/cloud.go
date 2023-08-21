package aws

import (
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/log"

	"github.com/golang/glog"
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
func NewCloud(region string, accountID string, svcNetwork string, vpcID string, useTGLongName bool) (Cloud, error) {
	// TODO: need to pass cfg CloudConfig later
	sess, _ := session.NewSession()

	sess.Handlers.Send.PushFront(func(r *request.Request) {

		glog.V(4).Info(fmt.Sprintf("Request: %s/%s, Payload: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params)))
	})

	sess.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {

			glog.ErrorDepth(2, fmt.Sprintf("Failed request: %s/%s, Payload: %s, Error: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Params), r.Error))
		} else {
			glog.V(4).Info(fmt.Sprintf("Response: %s/%s, Body: %s", r.ClientInfo.ServiceName, r.Operation.Name, log.Prettify(r.Data)))

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
