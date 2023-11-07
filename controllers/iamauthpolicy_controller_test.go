package controllers

import (
	"context"
	"fmt"
	"testing"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestIAMAuthControllerValidate(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	ctx := context.Background()
	mockClient := mock_client.NewMockClient(c)
	iamCtrl := &IAMAuthPolicyController{
		client: mockClient,
	}
	k8sPolicy := &anv1alpha1.IAMAuthPolicy{}
	k8sPolicy.Name = "iam-policy"

	t.Run("wrong group name", func(t *testing.T) {
		k8sPolicy.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group: "Wrong",
		}

		err := iamCtrl.validateSpec(ctx, k8sPolicy)
		assert.ErrorIs(t, err, TargetGroupNameErr)
	})

	t.Run("wrong kind", func(t *testing.T) {
		k8sPolicy.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group: gwv1alpha2.GroupName,
			Kind:  "Wrong",
		}

		err := iamCtrl.validateSpec(ctx, k8sPolicy)
		assert.ErrorIs(t, err, TargetKindErr)
	})

	t.Run("targetRef not found", func(t *testing.T) {
		k8sPolicy.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group: gwv1beta1.GroupName,
			Kind:  "Gateway",
			Name:  "GwName",
		}

		// mock for not found
		notFoundErr := apierrors.NewNotFound(schema.GroupResource{}, "")
		mockClient.EXPECT().Get(ctx, types.NamespacedName{
			Name: "GwName",
		}, gomock.Any()).Return(notFoundErr)

		err := iamCtrl.validateSpec(ctx, k8sPolicy)
		assert.ErrorIs(t, err, TargetRefNotExists)
	})

	t.Run("targetRef conflict", func(t *testing.T) {
		k8sPolicy.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group: gwv1beta1.GroupName,
			Kind:  "Gateway",
			Name:  "GwName",
		}

		// mock for not found
		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(nil)
		// mock for conflicts
		conflictPolicy := anv1alpha1.IAMAuthPolicy{}
		conflictPolicy.Name = "another-policy"
		conflictPolicy.Spec.TargetRef = k8sPolicy.Spec.TargetRef
		mockClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, l *anv1alpha1.IAMAuthPolicyList, _ ...client.ListOption) error {
				l.Items = append(l.Items, conflictPolicy)
				return nil
			})

		err := iamCtrl.validateSpec(ctx, k8sPolicy)
		assert.ErrorIs(t, err, TargetRefConflict)
	})

	t.Run("valid policy", func(t *testing.T) {
		k8sPolicy.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group: gwv1beta1.GroupName,
			Kind:  "Gateway",
			Name:  "GwName",
		}

		// mock for not found
		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(nil)
		// mock for conflicts
		mockClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).Return(nil)

		err := iamCtrl.validateSpec(ctx, k8sPolicy)
		assert.Nil(t, err)
	})
}

func TestIAMAuthPolicyValidationErrToStatus(t *testing.T) {

	type test struct {
		validationErr error
		wantReason    gwv1alpha2.PolicyConditionReason
	}

	tests := []test{
		{fmt.Errorf("%w", TargetGroupNameErr), gwv1alpha2.PolicyReasonInvalid},
		{fmt.Errorf("%w", TargetKindErr), gwv1alpha2.PolicyReasonInvalid},
		{fmt.Errorf("%w", TargetRefNotExists), gwv1alpha2.PolicyReasonTargetNotFound},
		{fmt.Errorf("%w", TargetRefConflict), gwv1alpha2.PolicyReasonConflicted},
		{nil, gwv1alpha2.PolicyReasonAccepted},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T", tt.validationErr), func(t *testing.T) {
			reason := validationErrToStatusReason(tt.validationErr)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}
