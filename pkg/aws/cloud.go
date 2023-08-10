package aws

import (
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
)

type Cloud interface {
	Lattice() services.Lattice
	EKS() services.EKS
}

// NewCloud constructs new Cloud implementation.
func NewCloud(log gwlog.Logger) (Cloud, error) {
	// TODO: need to pass cfg CloudConfig later
	sess, _ := session.NewSession()

	sess.Handlers.Send.PushFront(func(r *request.Request) {
		log.Debugw("request",
			"serviceName", r.ClientInfo.ServiceName,
			"operation", r.Operation.Name,
			"params", r.Params)
	})

	sess.Handlers.Complete.PushFront(func(r *request.Request) {
		if r.Error != nil {
			log.Errorw(r.Error.Error(),
				"serviceName", r.ClientInfo.ServiceName,
				"operation", r.Operation.Name, "params", r.Params)
		} else {
			log.Debugw("response",
				"serviceName", r.ClientInfo.ServiceName,
				"operation", r.Operation.Name,
			)
		}
	})

	return &defaultCloud{
		// TODO: service
		vpcLatticeSess: services.NewDefaultLattice(sess, config.Region),
		eksSess:        services.NewDefaultEKS(sess, config.Region),
	}, nil
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	vpcLatticeSess services.Lattice
	eksSess        services.EKS
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
