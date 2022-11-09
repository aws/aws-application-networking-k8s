package aws

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/log"

	"github.com/golang/glog"
)

type Cloud interface {
	Mercury() services.Mercury
	EKS() services.EKS
}

// NewCloud constructs new Cloud implementation.
func NewCloud() (Cloud, error) {
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
		vpcLatticeSess: services.NewDefaultMercury(sess, config.Region),
		eksSess:        services.NewDefaultEKS(sess, config.Region),
	}, nil
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	vpcLatticeSess services.Mercury
	eksSess        services.EKS
}

func (d *defaultCloud) Mercury() services.Mercury {
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
