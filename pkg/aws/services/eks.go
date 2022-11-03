package services

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/eks/eksiface"
)

type EKS interface {
	eksiface.EKSAPI
}

type defaultEKS struct {
	eksiface.EKSAPI
}

func NewDefaultEKS(sess *session.Session, region string) *defaultEKS {
	var eksSess eksiface.EKSAPI
	if region == "us-east-1" {
		eksSess = eks.New(sess, aws.NewConfig().WithRegion("us-east-1"))
	} else {
		eksSess = eks.New(sess, aws.NewConfig().WithRegion("us-west-2"))
	}
	return &defaultEKS{EKSAPI: eksSess}
}
