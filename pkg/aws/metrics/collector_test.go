package metrics

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
)

func Test_errorCodeFromErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "internal error",
			err:  errors.New("oops, some internal error"),
			want: "internal",
		},
		{
			name: "aws api error",
			err: &smithy.GenericAPIError{
				Code:    "NotFoundException",
				Message: "not found",
			},
			want: "NotFoundException",
		},
		{
			name: "wrapped aws api error",
			err: fmt.Errorf("wrapped: %w", &smithy.GenericAPIError{
				Code:    "ThrottlingException",
				Message: "rate exceeded",
			}),
			want: "ThrottlingException",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				return
			}
			got := errorCodeFromErr(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
