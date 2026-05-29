package services

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func Test_IsNotFoundError(t *testing.T) {
	err := errors.New("ERROR")
	nfErr := NewNotFoundError("type", "name")
	blankNfEff := ErrNotFound

	assert.False(t, IsNotFoundError(err))
	assert.False(t, IsNotFoundError(nil))
	assert.True(t, IsNotFoundError(nfErr))
	assert.True(t, IsNotFoundError(blankNfEff))
}

func TestServiceNetworkMatch(t *testing.T) {
	type SnSum = types.ServiceNetworkSummary

	type test struct {
		name       string
		inAllSn    []SnSum
		inNameOrId string
		outSn      *SnSum
		outErrType error
	}

	tests := []test{
		{
			name:       "not found",
			inAllSn:    []SnSum{},
			inNameOrId: "any",
			outErrType: ErrNotFound,
		},
		{
			name: "name conflict",
			inAllSn: []SnSum{
				{Name: aws.String("conflict")},
				{Name: aws.String("conflict")},
			},
			inNameOrId: "conflict",
			outErrType: ErrNameConflict,
		},
		{
			name: "name and id conflict",
			inAllSn: []SnSum{
				{Name: aws.String("sn-12345678326bb4a62")},
				{Id: aws.String("sn-12345678326bb4a62")},
			},
			inNameOrId: "sn-12345678326bb4a62",
			outErrType: ErrNameConflict,
		},
		{
			name: "id conflict",
			inAllSn: []SnSum{
				{Id: aws.String("sn-12345678326bb4a62")},
				{Id: aws.String("sn-12345678326bb4a62")},
			},
			inNameOrId: "sn-12345678326bb4a62",
			outErrType: ErrNameConflict,
		},
		{
			name: "name match",
			inAllSn: []SnSum{
				{Name: aws.String("not")},
				{Name: aws.String("match")},
			},
			inNameOrId: "match",
			outSn:      &SnSum{Name: aws.String("match")},
		},
		{
			name: "id match",
			inAllSn: []SnSum{
				{Id: aws.String("sn-12345678326bb4a62")},
				{Id: aws.String("sn-99999999999999999")},
			},
			inNameOrId: "sn-12345678326bb4a62",
			outSn: &types.ServiceNetworkSummary{
				Id: aws.String("sn-12345678326bb4a62"),
			},
		},
	}

	d := defaultLattice{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sn, err := d.serviceNetworkMatch(tt.inAllSn, tt.inNameOrId)
			assert.ErrorIs(t, err, tt.outErrType)
			if tt.outErrType == nil {
				assert.Equal(t,
					aws.ToString(tt.outSn.Name),
					aws.ToString(sn.Name),
				)
				assert.Equal(t,
					aws.ToString(tt.outSn.Id),
					aws.ToString(sn.Id),
				)
			}
		})
	}
}

func TestIsLocalResource(t *testing.T) {
	type test struct {
		name         string
		inArn        string
		inOwnAccount string
		outIsLocal   bool
		outIsErr     bool
	}

	tests := []test{
		{
			name:       "arn parse err",
			inArn:      "",
			outIsLocal: false,
			outIsErr:   true,
		},
		{
			name:         "arn matches ownAccount",
			inArn:        "arn:aws:vpc-lattice::123456789012:",
			inOwnAccount: "123456789012",
			outIsLocal:   true,
			outIsErr:     false,
		},
		{
			name:         "arn does not match ownAccount",
			inOwnAccount: "123456789012",
			inArn:        "arn:aws:vpc-lattice::111222333444:",
			outIsLocal:   false,
			outIsErr:     false,
		},
		{
			name:         "own account is empty",
			inOwnAccount: "",
			inArn:        "arn:aws:vpc-lattice::111222333444:",
			outIsLocal:   true,
			outIsErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := defaultLattice{ownAccount: tt.inOwnAccount}
			isLocal, err := d.isLocalResource(tt.inArn)
			if err != nil {
				assert.True(t, tt.outIsErr)
			} else {
				assert.Equal(t, tt.outIsLocal, isLocal)
			}
		})
	}
}
