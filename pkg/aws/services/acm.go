package services

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/acm/acmiface"
)

//go:generate mockgen -destination acm_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services ACM

var ErrACMAccessDenied = errors.New("ACM access denied: missing acm:ListCertificates permission")

type ACM interface {
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error)
}

type defaultACM struct {
	client acmiface.ACMAPI
}

func NewDefaultACM(sess *session.Session, region string) *defaultACM {
	return &defaultACM{
		client: acm.New(sess, aws.NewConfig().WithRegion(region).WithMaxRetries(20)),
	}
}

func (d *defaultACM) ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error) {
	var result []*acm.CertificateSummary
	err := d.client.ListCertificatesPagesWithContext(ctx, input, func(page *acm.ListCertificatesOutput, lastPage bool) bool {
		result = append(result, page.CertificateSummaryList...)
		return true
	})
	if err != nil {
		if isACMAccessDenied(err) {
			return nil, ErrACMAccessDenied
		}
		return nil, err
	}

	return result, nil
}

func isACMAccessDenied(err error) bool {
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == acm.ErrCodeAccessDeniedException
	}
	return false
}
