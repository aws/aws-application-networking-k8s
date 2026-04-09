package services

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
)

//go:generate mockgen -destination acm_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services ACM

var ErrACMAccessDenied = errors.New("ACM access denied: missing acm:ListCertificates permission")

type ACM interface {
	ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]acmtypes.CertificateSummary, error)
}

type defaultACM struct {
	client *acm.Client
}

func NewDefaultACM(cfg aws.Config, region string) *defaultACM {
	return &defaultACM{
		client: acm.NewFromConfig(cfg, func(o *acm.Options) {
			o.Region = region
			o.RetryMaxAttempts = 20
		}),
	}
}

func (d *defaultACM) ListCertificatesAsList(ctx context.Context, input *acm.ListCertificatesInput) ([]acmtypes.CertificateSummary, error) {
	var result []acmtypes.CertificateSummary
	paginator := acm.NewListCertificatesPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			if isACMAccessDenied(err) {
				return nil, ErrACMAccessDenied
			}
			return nil, err
		}
		result = append(result, page.CertificateSummaryList...)
	}
	return result, nil
}

func isACMAccessDenied(err error) bool {
	var ade *acmtypes.AccessDeniedException
	return errors.As(err, &ade)
}
