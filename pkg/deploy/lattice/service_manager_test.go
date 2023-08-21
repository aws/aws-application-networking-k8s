package lattice

import (
	"context"
	"errors"
	"testing"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestIntegCreateService(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	lat := services.NewMockLattice(c)
	cfg := mocks_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := mocks_aws.NewDefaultCloud(lat, nil, cfg)
	ds := latticestore.NewLatticeDataStore()
	ctx := context.Background()
	m := NewServiceManager(cl, ds)

	// Case for single service and single sn-svc association
	// Make sure that we send requests to Lattice for create Service and create Sn-Svc
	t.Run("create new service and sn assoc", func(t *testing.T) {
		ds.AddServiceNetwork("sn", cfg.AccountId, "sn-arn", "sn-id", "sn-status")
		lat.EXPECT().
			ListServicesAsList(gomock.Any(), gomock.Any()).Return([]*SvcSummary{}, nil)
		lat.EXPECT().
			CreateServiceWithContext(gomock.Any(), gomock.Any()).Return(&CreateSvcResp{
			Arn:      aws.String("arn"),
			DnsEntry: &vpclattice.DnsEntry{DomainName: aws.String("dns")},
			Id:       aws.String("svc-id"),
		}, nil)
		lat.EXPECT().
			CreateServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&CreateSnSvcAssocResp{
				Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
			}, nil)

		svc := &Service{
			Spec: latticemodel.ServiceSpec{
				Name:                "svc",
				Namespace:           "ns",
				ServiceNetworkNames: []string{"sn"},
				CustomerDomainName:  "dns",
				CustomerCertARN:     "cert-arn",
			},
		}
		status, err := m.Create(ctx, svc)
		assert.Nil(t, err)
		assert.Equal(t, "arn", status.Arn)
	})

	// Update is more complex than create, we need to apply diff for Sn-Svc associations
	// This test covers creation/deletion for multiple SN's
	// Service is associated with sn-keep and sn-delete,
	// for update we need to delete sn-delete and add sn-add
	t.Run("update sn-svc assocs", func(t *testing.T) {
		snNames := []string{"sn-keep", "sn-delete", "sn-add"}
		svc := &Service{
			Spec: latticemodel.ServiceSpec{
				Name:                "svc",
				Namespace:           "ns",
				ServiceNetworkNames: []string{snNames[0], snNames[2]},
			},
		}

		for _, sn := range snNames {
			ds.AddServiceNetwork(sn, cfg.AccountId, sn+"-arn", sn+"-id", sn+"-status")
		}

		lat.EXPECT().
			ListServicesAsList(gomock.Any(), gomock.Any()).
			Return([]*SvcSummary{{
				Arn:  aws.String("svc-arn"),
				Id:   aws.String("svc-id"),
				Name: aws.String(svc.LatticeName()),
			}}, nil)

		lat.EXPECT().
			ListServiceNetworkServiceAssociationsAsList(gomock.Any(), gomock.Any()).
			Return([]*SnSvcAssocSummary{
				{
					Arn:                aws.String(snNames[0] + "-arn"),
					Id:                 aws.String(snNames[0] + "-id"),
					ServiceNetworkName: &snNames[0],
					Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
				},
				{
					Arn:                aws.String(snNames[1] + "-arn"),
					Id:                 aws.String(snNames[1] + "-id"),
					ServiceNetworkName: &snNames[1],
					Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
				},
			}, nil)

		lat.EXPECT().
			CreateServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *CreateSnSvcAssocReq, _ ...interface{}) (*CreateSnSvcAssocResp, error) {
					assert.Equal(t, "sn-add-id", *req.ServiceNetworkIdentifier)
					return &CreateSnSvcAssocResp{Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive)}, nil
				})

		lat.EXPECT().
			ListTagsForResource(gomock.Any()).
			Return(&GetResourcesTagsResp{
				Tags: cl.NewTagsWithManagedBy(),
			}, nil)

		lat.EXPECT().
			DeleteServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *DelSnSvcAssocReq, _ ...interface{}) (*DelSnSvcAssocResp, error) {
					assert.Equal(t, "sn-delete-arn", *req.ServiceNetworkServiceAssociationIdentifier)
					return &DelSnSvcAssocResp{}, nil
				})

		status, err := m.Create(ctx, svc)
		assert.Nil(t, err)
		assert.Equal(t, "svc-arn", status.Arn)
	})

	// We should not delete resources that were created outside of Gateway-API
	// For example when service is shared with another account / SN
	// Check for  DeleteServiceNetworkServiceAssociationWithContext happens implicitly
	// if deletion happens gomock will throw exception unexpected call
	t.Run("dont delete sn-svc assoc not managed by gateway-api", func(t *testing.T) {
		svc := &Service{
			Spec: latticemodel.ServiceSpec{
				Name:                "svc",
				Namespace:           "ns",
				ServiceNetworkNames: []string{"sn"},
			},
		}

		ds.AddServiceNetwork("sn", cfg.AccountId, "sn-arn", "sn-id", "sn-status")

		lat.EXPECT().
			ListServicesAsList(gomock.Any(), gomock.Any()).
			Return([]*SvcSummary{{
				Arn:  aws.String("svc-arn"),
				Id:   aws.String("svc-id"),
				Name: aws.String(svc.LatticeName()),
			}}, nil)

		lat.EXPECT().
			ListServiceNetworkServiceAssociationsAsList(gomock.Any(), gomock.Any()).
			Return([]*SnSvcAssocSummary{
				{
					Arn:                aws.String("assoc-arn"),
					Id:                 aws.String("assoc-id"),
					ServiceNetworkName: aws.String("sn"),
					Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
				},
				{
					Arn:                aws.String("foreign-assoc-arn"),
					Id:                 aws.String("foreign-assoc-id"),
					ServiceNetworkName: aws.String("foreign-sn"),
					Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
				},
			}, nil)

		lat.EXPECT().
			ListTagsForResource(gomock.Any()).
			DoAndReturn(
				func(req *GetResourcesTagsReq) (*GetResourcesTagsResp, error) {
					var tags mocks_aws.Tags
					if *req.ResourceArn == "foreign-assoc-arn" {
						tags = cl.NewTags()
					} else {
						tags = cl.NewTagsWithManagedBy()
					}
					return &GetResourcesTagsResp{Tags: tags}, nil
				})

		status, err := m.Create(ctx, svc)
		assert.Nil(t, err)
		assert.Equal(t, "svc-arn", status.Arn)
	})
}

func TestCreateSvcReq(t *testing.T) {
	cfg := mocks_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := mocks_aws.NewDefaultCloud(nil, nil, cfg)
	ds := latticestore.NewLatticeDataStore()
	m := NewServiceManager(cl, ds)

	spec := latticemodel.ServiceSpec{
		Name:               "name",
		Namespace:          "ns",
		CustomerDomainName: "dns",
		CustomerCertARN:    "cert-arn",
	}

	svcModel := &latticemodel.Service{
		Spec: spec,
	}

	req := m.newCreateSvcReq(svcModel)

	assert.Equal(t, *req.Name, svcModel.LatticeName())
	assert.Equal(t, *req.CustomDomainName, spec.CustomerDomainName)
	assert.Equal(t, *req.CertificateArn, spec.CustomerCertARN)
	assert.True(t, cl.IsTagManagedBy(req.Tags))

}

func TestSvcStatusFromCreateSvcResp(t *testing.T) {
	resp := &CreateSvcResp{
		Arn: aws.String("arn"),
		DnsEntry: &vpclattice.DnsEntry{
			DomainName: aws.String("dns"),
		},
		Id: aws.String("id"),
	}

	status := svcStatusFromCreateSvcResp(resp)

	assert.Equal(t, *resp.Arn, status.Arn)
	assert.Equal(t, *resp.Id, status.Id)
	assert.Equal(t, *resp.DnsEntry.DomainName, status.Dns)
}

func TestHandleSnSvcAssocResp(t *testing.T) {

	t.Run("assoc status active", func(t *testing.T) {
		resp := &CreateSnSvcAssocResp{
			Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
		}
		err := handleSnSvcAssocResp(resp)
		assert.Nil(t, err)
	})

	t.Run("assoc status non active", func(t *testing.T) {
		resp := &CreateSnSvcAssocResp{
			Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress),
		}
		err := handleSnSvcAssocResp(resp)
		assert.True(t, errors.Is(err, RetryErr))
	})

}

func TestSnSvcAssocsDiff(t *testing.T) {

	t.Run("no diff", func(t *testing.T) {
		svc := &Service{Spec: latticemodel.ServiceSpec{
			ServiceNetworkNames: []string{"sn"},
		}}
		assocs := []*SnSvcAssocSummary{{ServiceNetworkName: aws.String("sn")}}
		c, d, err := snSvcAssocsDiff(svc, assocs)
		assert.Nil(t, err)
		assert.Equal(t, 0, len(c))
		assert.Equal(t, 0, len(d))
	})

	t.Run("only create", func(t *testing.T) {
		svc := &Service{Spec: latticemodel.ServiceSpec{
			ServiceNetworkNames: []string{"sn1", "sn2"},
		}}
		assocs := []*SnSvcAssocSummary{}
		c, d, _ := snSvcAssocsDiff(svc, assocs)
		assert.Equal(t, 2, len(c))
		assert.Equal(t, 0, len(d))
	})

	t.Run("only delete", func(t *testing.T) {
		svc := &Service{}
		assocs := []*SnSvcAssocSummary{
			{ServiceNetworkName: aws.String("sn1")},
			{ServiceNetworkName: aws.String("sn2")},
		}
		c, d, _ := snSvcAssocsDiff(svc, assocs)
		assert.Equal(t, 0, len(c))
		assert.Equal(t, 2, len(d))
	})

	t.Run("create and delete", func(t *testing.T) {
		svc := &Service{Spec: latticemodel.ServiceSpec{
			ServiceNetworkNames: []string{"sn1", "sn2", "sn3"},
		}}
		assocs := []*SnSvcAssocSummary{
			{ServiceNetworkName: aws.String("sn1")},
			{ServiceNetworkName: aws.String("sn4")},
		}
		c, d, _ := snSvcAssocsDiff(svc, assocs)
		assert.Equal(t, 2, len(c))
		assert.Equal(t, 1, len(d))
	})

	t.Run("retry error on create assoc that in deletion state", func(t *testing.T) {
		svc := &Service{Spec: latticemodel.ServiceSpec{
			ServiceNetworkNames: []string{"sn"},
		}}
		assocs := []*SnSvcAssocSummary{{
			ServiceNetworkName: aws.String("sn"),
			Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress),
		}}
		_, _, err := snSvcAssocsDiff(svc, assocs)
		assert.True(t, errors.Is(err, RetryErr))
	})
}
