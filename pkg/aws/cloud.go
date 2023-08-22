package aws

import (
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
)

//go:generate mockgen -destination cloud_mocks.go -package aws github.com/aws/aws-application-networking-k8s/pkg/aws Cloud

type CloudConfig struct {
	VpcId     string
	AccountId string
}

type Cloud interface {
	Config() CloudConfig
	Lattice() services.Lattice
	EKS() services.EKS
}

// NewCloud constructs new Cloud implementation.
func NewCloud(log gwlog.Logger, cfg CloudConfig) (Cloud, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

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

	lattice := services.NewDefaultLattice(sess, config.Region)
	eks := services.NewDefaultEKS(sess, config.Region)
	cl := NewDefaultCloud(lattice, eks, cfg)
	return cl, nil
}

// Used in testing and mocks
func NewDefaultCloud(lattice services.Lattice, eks services.EKS, cfg CloudConfig) Cloud {
	return &defaultCloud{cfg, lattice, eks}
}

type defaultCloud struct {
	cfg     CloudConfig
	lattice services.Lattice
	eks     services.EKS
}

func (c *defaultCloud) Lattice() services.Lattice {
	return c.lattice
}

func (c *defaultCloud) EKS() services.EKS {
	return c.eks
}

func (c *defaultCloud) Config() CloudConfig {
	return c.cfg
}

func (d *defaultCloud) GetEKSClusterVPC(name string) string {
	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}

	result, err := d.eks.DescribeCluster(input)

	if err != nil {
		return ""
	}
	return result.String()
}
