package controllers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
)

const testLatticeControllerName = config.LatticeGatewayControllerName

func newSchemeForSNTest() *runtime.Scheme {
	s := runtime.NewScheme()
	clientgoscheme.AddToScheme(s)
	gwv1.Install(s)

	gv := schema.GroupVersion{Group: anv1alpha1.GroupName, Version: "v1alpha1"}
	s.AddKnownTypes(gv, &anv1alpha1.ServiceNetwork{}, &anv1alpha1.ServiceNetworkList{})
	metav1.AddToGroupVersion(s, gv)
	return s
}

func TestServiceNetworkReconciler_UpsertCreatesNew(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := newSchemeForSNTest()
	sn := &anv1alpha1.ServiceNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "my-network"},
	}
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(sn).
		WithStatusSubresource(&anv1alpha1.ServiceNetwork{}).
		Build()

	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	mockSNManager.EXPECT().Upsert(gomock.Any(), "my-network", gomock.Any()).
		Return(model.ServiceNetworkStatus{
			ServiceNetworkARN: "arn:aws:vpc-lattice:us-west-2:123456789:servicenetwork/sn-123",
			ServiceNetworkID:  "sn-123",
		}, nil)

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &serviceNetworkReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		snManager:        mockSNManager,
		eventRecorder:    mockEventRecorder,
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-network"}})
	assert.Nil(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)

	// Verify status was updated
	updated := &anv1alpha1.ServiceNetwork{}
	k8sClient.Get(ctx, types.NamespacedName{Name: "my-network"}, updated)
	assert.Equal(t, "arn:aws:vpc-lattice:us-west-2:123456789:servicenetwork/sn-123", updated.Status.ServiceNetworkARN)
	assert.Equal(t, "sn-123", updated.Status.ServiceNetworkID)
}

func TestServiceNetworkReconciler_UpsertError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := newSchemeForSNTest()
	sn := &anv1alpha1.ServiceNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "my-network"},
	}
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(sn).
		WithStatusSubresource(&anv1alpha1.ServiceNetwork{}).
		Build()

	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	mockSNManager.EXPECT().Upsert(gomock.Any(), "my-network", gomock.Any()).
		Return(model.ServiceNetworkStatus{}, errors.New("lattice error"))

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &serviceNetworkReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		snManager:        mockSNManager,
		eventRecorder:    mockEventRecorder,
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-network"}})
	// HandleReconcileError requeues on error
	assert.NotNil(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
}

func TestServiceNetworkReconciler_DeleteSuccess(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := newSchemeForSNTest()
	now := metav1.Now()
	sn := &anv1alpha1.ServiceNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-network",
			DeletionTimestamp: &now,
			Finalizers:        []string{serviceNetworkFinalizer},
		},
	}
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(sn).
		WithStatusSubresource(&anv1alpha1.ServiceNetwork{}).
		Build()

	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	mockSNManager.EXPECT().Delete(gomock.Any(), "my-network").Return(nil)

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().RemoveFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &serviceNetworkReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		snManager:        mockSNManager,
		eventRecorder:    mockEventRecorder,
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-network"}})
	assert.Nil(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
}

func TestServiceNetworkReconciler_DeleteBlockedByGateway(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := newSchemeForSNTest()
	now := metav1.Now()
	sn := &anv1alpha1.ServiceNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-network",
			DeletionTimestamp: &now,
			Finalizers:        []string{serviceNetworkFinalizer},
		},
		Status: anv1alpha1.ServiceNetworkStatus{
			ServiceNetworkARN: "arn:aws:vpc-lattice:us-west-2:123456789:servicenetwork/sn-123",
			ServiceNetworkID:  "sn-123",
		},
	}

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: testLatticeControllerName,
		},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-network",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(sn, gwClass, gw).
		WithStatusSubresource(&anv1alpha1.ServiceNetwork{}).
		Build()

	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	// Delete should NOT be called since gateway blocks it

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	// RemoveFinalizers should NOT be called

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &serviceNetworkReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		snManager:        mockSNManager,
		eventRecorder:    mockEventRecorder,
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-network"}})
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "Cannot delete")
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
}

func TestServiceNetworkReconciler_DeleteConflictError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := newSchemeForSNTest()
	now := metav1.Now()
	sn := &anv1alpha1.ServiceNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-network",
			DeletionTimestamp: &now,
			Finalizers:        []string{serviceNetworkFinalizer},
		},
		Status: anv1alpha1.ServiceNetworkStatus{
			ServiceNetworkARN: "arn:aws:vpc-lattice:us-west-2:123456789:servicenetwork/sn-123",
			ServiceNetworkID:  "sn-123",
		},
	}
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(sn).
		WithStatusSubresource(&anv1alpha1.ServiceNetwork{}).
		Build()

	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	mockSNManager.EXPECT().Delete(gomock.Any(), "my-network").
		Return(fmt.Errorf("ConflictException: service network has VPC(s) associated"))

	mockFinalizer := k8s.NewMockFinalizerManager(c)

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	r := &serviceNetworkReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		snManager:        mockSNManager,
		eventRecorder:    mockEventRecorder,
	}

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "my-network"}})
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "ConflictException")
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
}

func TestServiceNetworkReconciler_NotFound(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := newSchemeForSNTest()
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		Build()

	mockSNManager := deploy.NewMockServiceNetworkManager(c)
	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockEventRecorder := mock_client.NewMockEventRecorder(c)

	r := &serviceNetworkReconciler{
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		finalizerManager: mockFinalizer,
		snManager:        mockSNManager,
		eventRecorder:    mockEventRecorder,
	}

	// Reconcile a non-existent resource — should return no error (IgnoreNotFound)
	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist"}})
	assert.Nil(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
}
