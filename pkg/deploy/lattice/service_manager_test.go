package lattice

import (
	"context"
	"errors"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestServiceManagerInteg(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	cfg := pkg_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := pkg_aws.NewDefaultCloud(mockLattice, cfg)
	ctx := context.Background()
	m := NewServiceManager(gwlog.FallbackLogger, cl)

	// Case for single service and single sn-svc association
	// Make sure that we send requests to Lattice for create Service and create Sn-Svc
	t.Run("create new service and association", func(t *testing.T) {

		svc := &Service{
			Spec: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "svc",
					RouteNamespace: "ns",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{"sn"},
				CustomerDomainName:  "dns",
				CustomerCertARN:     "cert-arn",
			},
		}

		// service does not exist in lattice
		mockLattice.EXPECT().
			FindService(gomock.Any(), gomock.Any()).
			Return(nil, &mocks.NotFoundError{}).
			Times(1)

		// assert that we call create service
		mockLattice.EXPECT().
			CreateServiceWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *CreateSvcReq, _ ...interface{}) (*CreateSvcResp, error) {
					assert.Equal(t, svc.LatticeServiceName(), *req.Name)
					return &CreateSvcResp{
						Arn:      aws.String("arn"),
						DnsEntry: &vpclattice.DnsEntry{DomainName: aws.String("dns")},
						Id:       aws.String("svc-id"),
					}, nil
				}).
			Times(1)

		// assert that we call create association
		mockLattice.EXPECT().
			CreateServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *CreateSnSvcAssocReq, _ ...interface{}) (*CreateSnSvcAssocResp, error) {
					assert.Equal(t, "sn-id", *req.ServiceNetworkIdentifier)
					assert.Contains(t, req.Tags, pkg_aws.TagManagedBy)
					return &CreateSnSvcAssocResp{
						Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
					}, nil
				}).
			Times(1)

		// expect a call to find the service network
		mockLattice.EXPECT().
			FindServiceNetwork(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(ctx context.Context, name string, accountId string) (*mocks.ServiceNetworkInfo, error) {
					return &mocks.ServiceNetworkInfo{
						SvcNetwork: vpclattice.ServiceNetworkSummary{
							Arn:  aws.String("sn-arn"),
							Id:   aws.String("sn-id"),
							Name: aws.String(name),
						},
						Tags: nil,
					}, nil
				}).
			Times(1)

		status, err := m.Upsert(ctx, svc)
		assert.Nil(t, err)
		assert.Equal(t, "arn", status.Arn)
	})

	// Update is more complex than create, we need to apply diff for Sn-Svc associations
	// This test covers creation/deletion for multiple SN's
	// sn-keep - no changes
	// sn-delete - managed by gateway and want to delete
	// sn-create - managed by gateway and want to create
	// sn-foreign - not managed by gateway and exists in lattice (should keep)
	t.Run("update service's associations", func(t *testing.T) {
		snKeep := "sn-keep"
		snDelete := "sn-delete"
		snAdd := "sn-add"
		snForeign := "sn-foreign"

		svc := &Service{
			Spec: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "svc",
					RouteNamespace: "ns",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{snKeep, snAdd},
			},
		}

		// service exists in lattice
		mockLattice.EXPECT().
			FindService(gomock.Any(), gomock.Any()).
			Return(&vpclattice.ServiceSummary{
				Arn:  aws.String("svc-arn"),
				Id:   aws.String("svc-id"),
				Name: aws.String(svc.LatticeServiceName()),
			}, nil).
			Times(1)

		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
				return &vpclattice.ListTagsForResourceOutput{
					Tags: cl.DefaultTagsMergedWith(svc.Spec.ToTags()),
				}, nil
			}).
			Times(1) // for service only

		// 3 associations exist in lattice: keep, delete, and foreign
		mockLattice.EXPECT().
			ListServiceNetworkServiceAssociationsAsList(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ListSnSvcAssocsReq) ([]*SnSvcAssocSummary, error) {
				assocs := []*SnSvcAssocSummary{}
				for _, sn := range []string{snKeep, snDelete, snForeign} {
					assocs = append(assocs, &SnSvcAssocSummary{
						Arn:                aws.String(sn + "-arn"),
						Id:                 aws.String(sn + "-id"),
						ServiceNetworkName: aws.String(sn),
						Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
					})
				}
				return assocs, nil
			}).
			Times(1)

		// return managed by gateway controller tags for all associations except for foreign
		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
				if *req.ResourceArn == snForeign+"-arn" {
					return &vpclattice.ListTagsForResourceOutput{}, nil
				} else {
					return &vpclattice.ListTagsForResourceOutput{
						Tags: cl.DefaultTags(),
					}, nil
				}
			}).
			Times(2) // delete and foreign

		// assert we call create and delete associations on managed resources
		mockLattice.EXPECT().
			CreateServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *CreateSnSvcAssocReq, _ ...interface{}) (*CreateSnSvcAssocResp, error) {
					assert.Equal(t, snAdd+"-id", *req.ServiceNetworkIdentifier)
					return &CreateSnSvcAssocResp{Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive)}, nil
				}).
			Times(1)

		// make sure we call delete only on managed resource, only snDelete should be deleted
		mockLattice.EXPECT().
			DeleteServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *DelSnSvcAssocReq, _ ...interface{}) (*DelSnSvcAssocResp, error) {
					assert.Equal(t, snDelete+"-arn", *req.ServiceNetworkServiceAssociationIdentifier)
					return &DelSnSvcAssocResp{}, nil
				}).
			Times(1)

		// expect calls to find the service network
		mockLattice.EXPECT().
			FindServiceNetwork(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(ctx context.Context, name string, accountId string) (*mocks.ServiceNetworkInfo, error) {
					return &mocks.ServiceNetworkInfo{
						SvcNetwork: vpclattice.ServiceNetworkSummary{
							Arn:  aws.String(name + "-arn"),
							Id:   aws.String(name + "-id"),
							Name: aws.String(name),
						},
						Tags: nil,
					}, nil
				}).
			AnyTimes()

		status, err := m.Upsert(ctx, svc)
		assert.Nil(t, err)
		assert.Equal(t, "svc-arn", status.Arn)
	})

	t.Run("backfilling service tags", func(t *testing.T) {
		svc := &Service{
			Spec: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "svc",
					RouteNamespace: "ns",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{"sn"},
			},
		}

		// service exists in lattice
		mockLattice.EXPECT().
			FindService(gomock.Any(), gomock.Any()).
			Return(&vpclattice.ServiceSummary{
				Arn:  aws.String("svc-arn"),
				Id:   aws.String("svc-id"),
				Name: aws.String(svc.LatticeServiceName()),
			}, nil).
			Times(1)

		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
				return &vpclattice.ListTagsForResourceOutput{
					Tags: map[string]*string{},
				}, nil
			}).
			Times(1)

		mockLattice.EXPECT().TagResourceWithContext(gomock.Any(), gomock.Eq(&vpclattice.TagResourceInput{
			ResourceArn: aws.String("svc-arn"),
			Tags:        cl.DefaultTags(),
		})).Times(1)

		mockLattice.EXPECT().TagResourceWithContext(gomock.Any(), gomock.Eq(&vpclattice.TagResourceInput{
			ResourceArn: aws.String("svc-arn"),
			Tags:        svc.Spec.ToTags(),
		})).Times(1)

		mockLattice.EXPECT().ListServiceNetworkServiceAssociationsAsList(gomock.Any(), gomock.Any()).Times(1)
		mockLattice.EXPECT().
			CreateServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(
				func(_ context.Context, req *CreateSnSvcAssocReq, _ ...interface{}) (*CreateSnSvcAssocResp, error) {
					return &CreateSnSvcAssocResp{Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive)}, nil
				}).
			Times(1)

		status, err := m.Upsert(ctx, svc)
		assert.Nil(t, err)
		assert.Equal(t, "svc-arn", status.Arn)
	})

	t.Run("delete service and association", func(t *testing.T) {
		svc := &Service{
			Spec: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "svc",
					RouteNamespace: "ns",
				},
				ServiceNetworkNames: []string{"sn"},
			},
		}

		// service exists
		mockLattice.EXPECT().
			FindService(gomock.Any(), gomock.Any()).
			Return(&vpclattice.ServiceSummary{
				Arn:  aws.String("svc-arn"),
				Id:   aws.String("svc-id"),
				Name: aws.String(svc.LatticeServiceName()),
			}, nil)

		mockLattice.EXPECT().ListTagsForResourceWithContext(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
				return &vpclattice.ListTagsForResourceOutput{
					Tags: cl.DefaultTagsMergedWith(svc.Spec.ToTags()),
				}, nil
			}).
			Times(1) // for service only

		mockLattice.EXPECT().
			ListServiceNetworkServiceAssociationsAsList(gomock.Any(), gomock.Any()).
			Return([]*SnSvcAssocSummary{
				{
					Arn:                aws.String("assoc-arn"),
					Id:                 aws.String("assoc-id"),
					ServiceNetworkName: aws.String("sn"),
					Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
				},
			}, nil)

		// assert we delete association and service
		mockLattice.EXPECT().
			DeleteServiceNetworkServiceAssociationWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			Times(1)

		mockLattice.EXPECT().
			DeleteServiceWithContext(gomock.Any(), gomock.Any()).Return(nil, nil).
			Times(1)

		err := m.Delete(ctx, svc)
		assert.Nil(t, err)
	})

}

func TestCreateSvcReq(t *testing.T) {
	cfg := pkg_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := pkg_aws.NewDefaultCloud(nil, cfg)
	m := NewServiceManager(gwlog.FallbackLogger, cl)

	spec := model.ServiceSpec{
		ServiceTagFields: model.ServiceTagFields{
			RouteName:      "svc",
			RouteNamespace: "ns",
			RouteType:      core.HttpRouteType,
		},
		CustomerDomainName: "dns",
		CustomerCertARN:    "cert-arn",
	}

	svcModel := &model.Service{
		Spec: spec,
	}

	req := m.newCreateSvcReq(svcModel)

	assert.Equal(t, req.Tags, cl.DefaultTagsMergedWith(spec.ToTags()))
	assert.Equal(t, *req.Name, svcModel.LatticeServiceName())
	assert.Equal(t, *req.CustomDomainName, spec.CustomerDomainName)
	assert.Equal(t, *req.CertificateArn, spec.CustomerCertARN)

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
		err := handleCreateAssociationResp(resp)
		assert.Nil(t, err)
	})

	t.Run("assoc status non active", func(t *testing.T) {
		resp := &CreateSnSvcAssocResp{
			Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress),
		}
		err := handleCreateAssociationResp(resp)
		assert.True(t, errors.Is(err, RetryErr))
	})

}

func TestSnSvcAssocsDiff(t *testing.T) {

	t.Run("no diff", func(t *testing.T) {
		svc := &Service{Spec: model.ServiceSpec{
			ServiceNetworkNames: []string{"sn"},
		}}
		assocs := []*SnSvcAssocSummary{{ServiceNetworkName: aws.String("sn")}}
		c, d, err := associationsDiff(svc, assocs)
		assert.Nil(t, err)
		assert.Equal(t, 0, len(c))
		assert.Equal(t, 0, len(d))
	})

	t.Run("only create", func(t *testing.T) {
		svc := &Service{Spec: model.ServiceSpec{
			ServiceNetworkNames: []string{"sn1", "sn2"},
		}}
		assocs := []*SnSvcAssocSummary{}
		c, d, _ := associationsDiff(svc, assocs)
		assert.Equal(t, 2, len(c))
		assert.Equal(t, 0, len(d))
	})

	t.Run("only delete", func(t *testing.T) {
		svc := &Service{}
		assocs := []*SnSvcAssocSummary{
			{ServiceNetworkName: aws.String("sn1")},
			{ServiceNetworkName: aws.String("sn2")},
		}
		c, d, _ := associationsDiff(svc, assocs)
		assert.Equal(t, 0, len(c))
		assert.Equal(t, 2, len(d))
	})

	t.Run("create and delete", func(t *testing.T) {
		svc := &Service{Spec: model.ServiceSpec{
			ServiceNetworkNames: []string{"sn1", "sn2", "sn3"},
		}}
		assocs := []*SnSvcAssocSummary{
			{ServiceNetworkName: aws.String("sn1")},
			{ServiceNetworkName: aws.String("sn4")},
		}
		c, d, _ := associationsDiff(svc, assocs)
		assert.Equal(t, 2, len(c))
		assert.Equal(t, 1, len(d))
	})

	t.Run("retry error on create assoc that in deletion state", func(t *testing.T) {
		svc := &Service{Spec: model.ServiceSpec{
			ServiceNetworkNames: []string{"sn"},
		}}
		assocs := []*SnSvcAssocSummary{{
			ServiceNetworkName: aws.String("sn"),
			Status:             aws.String(vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress),
		}}
		_, _, err := associationsDiff(svc, assocs)
		assert.True(t, errors.Is(err, RetryErr))
	})

}
