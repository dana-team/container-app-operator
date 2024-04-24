package utils_test

import (
	"testing"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	"github.com/stretchr/testify/assert"
)

func TestFilterKeysWithoutPrefix(t *testing.T) {
	object := map[string]string{
		"prefix_key1": "value1",
		"key2":        "value2",
		"prefix_key3": "value3",
	}
	prefix := "prefix_"
	expected := map[string]string{
		"prefix_key1": "value1",
		"prefix_key3": "value3",
	}

	assert.Equal(t, expected, utils.FilterKeysWithoutPrefix(object, prefix))
}
