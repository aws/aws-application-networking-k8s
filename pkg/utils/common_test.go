package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_ArnToAccountId(t *testing.T) {
	emptyArn, err1 := ArnToAccountId("")
	assert.Equal(t, "", emptyArn)
	assert.Nil(t, err1)

	actualArn, err2 := ArnToAccountId("arn:aws:vpc-lattice:us-east-2:12345:type/id")
	assert.Equal(t, "12345", actualArn)
	assert.Nil(t, err2)

	_, err3 := ArnToAccountId("not-valid-arn:aws:us-east-2:12345:type/id")
	assert.NotNil(t, err3)

	_, err4 := ArnToAccountId("foo")
	assert.NotNil(t, err4)
}
