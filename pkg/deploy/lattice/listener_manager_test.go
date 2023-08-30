package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"k8s.io/apimachinery/pkg/types"

	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

var namespaceName = types.NamespacedName{
	Namespace: "default",
	Name:      "test",
}
var listenersummarys = []struct {
	Arn      string
	Id       string
	Name     string
	Port     int64
	Protocol string
}{
	{
		Arn:      "arn-1",
		Id:       "id-1",
		Name:     namespaceName.Name,
		Port:     80,
		Protocol: "HTTP",
	},
	{
		Arn:      "arn-2",
		Id:       "Id-2",
		Name:     namespaceName.Name,
		Port:     443,
		Protocol: "HTTPS",
	},
}
var summarys = []vpclattice.ListenerSummary{
	{
		Arn:      &listenersummarys[0].Arn,
		Id:       &listenersummarys[0].Id,
		Name:     &listenersummarys[0].Name,
		Port:     &listenersummarys[0].Port,
		Protocol: &listenersummarys[0].Protocol,
	},
	{
		Arn:      &listenersummarys[1].Arn,
		Id:       &listenersummarys[1].Id,
		Name:     &listenersummarys[1].Name,
		Port:     &listenersummarys[1].Port,
		Protocol: &listenersummarys[1].Protocol,
	},
}
var listenerList = vpclattice.ListListenersOutput{
	Items: []*vpclattice.ListenerSummary{
		&summarys[0],
		&summarys[1],
	},
}

func Test_AddListener(t *testing.T) {

	tests := []struct {
		name        string
		isUpdate    bool
		noServiceID bool
	}{
		{
			name:        "add listner",
			isUpdate:    false,
			noServiceID: false,
		},

		{
			name:        "update listner",
			isUpdate:    true,
			noServiceID: false,
		},

		{
			name:        "add listner, no service ID",
			isUpdate:    false,
			noServiceID: true,
		},
	}

	for _, tt := range tests {
		fmt.Printf("testing >>>>>>> %v \n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		mockVpcLatticeSess := mocks.NewMockLattice(c)

		mockCloud := mocks_aws.NewMockCloud(c)

		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		latticeDataStore := latticestore.NewLatticeDataStore()
		listenerManager := NewListenerManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)

		var serviceID = "serviceID"
		var serviceARN = "serviceARN"
		var serviceDNS = "DNS-test"

		if !tt.noServiceID {
			// Add service to ds
			latticeDataStore.AddLatticeService(namespaceName.Name, namespaceName.Namespace, serviceARN, serviceID, serviceDNS)

		}

		stack := core.NewDefaultStack(core.StackID(namespaceName))

		action := latticemodel.DefaultAction{
			Is_Import:               false,
			BackendServiceName:      "tg-test",
			BackendServiceNamespace: "tg-default",
		}

		listenerResourceName := fmt.Sprintf("%s-%s-%d-%s", namespaceName.Name, namespaceName.Namespace,
			int64(listenersummarys[0].Port), "HTTP")

		listener := latticemodel.NewListener(stack, listenerResourceName, int64(listenersummarys[0].Port), "HTTP",
			namespaceName.Name, namespaceName.Namespace, action)

		listenerOutput := vpclattice.CreateListenerOutput{}
		listenerInput := vpclattice.CreateListenerInput{}

		defaultStatus := aws.Int64(404)

		defaultResp := vpclattice.FixedResponseAction{
			StatusCode: defaultStatus,
		}
		defaultAction := vpclattice.RuleAction{
			FixedResponse: &defaultResp,
		}
		//listenerARN := "listener-ARN"
		//listenerID := "listener-ID"
		if !tt.noServiceID && !tt.isUpdate {

			listername := k8sLatticeListenerName(namespaceName.Name, namespaceName.Namespace,
				int(listenersummarys[0].Port), listenersummarys[0].Protocol)
			listenerInput = vpclattice.CreateListenerInput{
				DefaultAction:     &defaultAction,
				Name:              &listername,
				ServiceIdentifier: &serviceID,
				Protocol:          aws.String("HTTP"),
				Port:              aws.Int64(listenersummarys[0].Port),
			}
			listenerOutput = vpclattice.CreateListenerOutput{
				Arn:           &listenersummarys[0].Arn,
				DefaultAction: &defaultAction,
				Id:            &listenersummarys[0].Id,
			}
			mockVpcLatticeSess.EXPECT().CreateListener(&listenerInput).Return(&listenerOutput, nil)
		}

		if !tt.noServiceID {

			listenerListInput := vpclattice.ListListenersInput{
				ServiceIdentifier: aws.String(serviceID),
			}

			listenerOutput := vpclattice.ListListenersOutput{}

			if tt.isUpdate {

				listenerOutput = listenerList

			}

			mockVpcLatticeSess.EXPECT().ListListeners(&listenerListInput).Return(&listenerOutput, nil)
		}
		resp, err := listenerManager.Create(ctx, listener)

		if !tt.noServiceID {
			assert.NoError(t, err)

			assert.Equal(t, resp.ListenerARN, listenersummarys[0].Arn)
			assert.Equal(t, resp.ListenerID, listenersummarys[0].Id)
			assert.Equal(t, resp.Name, namespaceName.Name)
			assert.Equal(t, resp.Namespace, namespaceName.Namespace)
			assert.Equal(t, resp.Port, listenersummarys[0].Port)
			assert.Equal(t, resp.Protocol, "HTTP")
		}

		fmt.Printf("listener create : resp %v, err %v, listernerOutput %v\n", resp, err, listenerOutput)

		if tt.noServiceID {
			assert.NotNil(t, err)
		}
	}

}

func Test_ListListener(t *testing.T) {

	tests := []struct {
		Name   string
		mgrErr error
	}{
		{
			Name:   "listener LIST API call ok",
			mgrErr: nil,
		},
		{
			Name:   "listener List API call return NOK",
			mgrErr: errors.New("call failed"),
		},
	}

	for _, tt := range tests {

		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		mockVpcLatticeSess := mocks.NewMockLattice(c)

		mockCloud := mocks_aws.NewMockCloud(c)

		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		latticeDataStore := latticestore.NewLatticeDataStore()
		listenerManager := NewListenerManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)

		serviceID := "service1-ID"
		listenerListInput := vpclattice.ListListenersInput{
			ServiceIdentifier: aws.String(serviceID),
		}
		mockVpcLatticeSess.EXPECT().ListListeners(&listenerListInput).Return(&listenerList, tt.mgrErr)

		resp, err := listenerManager.List(ctx, serviceID)

		fmt.Printf("listener list :%v, err: %v \n", resp, err)

		if err == nil {
			var i = 0
			for _, rsp := range resp {
				assert.Equal(t, *rsp.Arn, *listenerList.Items[i].Arn)
				i++

			}

		} else {

			assert.Equal(t, err, tt.mgrErr)
		}
	}

}

func Test_DeleteListerner(t *testing.T) {

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockVpcLatticeSess := mocks.NewMockLattice(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	serviceID := "service1-ID"
	listenerID := "listener-ID"

	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(serviceID),
		ListenerIdentifier: aws.String(listenerID),
	}

	latticeDataStore := latticestore.NewLatticeDataStore()

	listenerDeleteOuput := vpclattice.DeleteListenerOutput{}
	mockVpcLatticeSess.EXPECT().DeleteListener(&listenerDeleteInput).Return(&listenerDeleteOuput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	listenerManager := NewListenerManager(gwlog.FallbackLogger, mockCloud, latticeDataStore)

	listenerManager.Delete(ctx, listenerID, serviceID)

}
