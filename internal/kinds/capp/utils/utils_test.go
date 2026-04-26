package utils_test

import (
	"testing"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	"github.com/stretchr/testify/require"
)

func TestExcludeKeysWithPrefix(t *testing.T) {
	object := map[string]string{
		"prefix_key1": "value1",
		"key2":        "value2",
		"prefix_key3": "value3",
	}
	require.Equal(t, map[string]string{"key2": "value2"}, utils.ExcludeKeysWithPrefix(object, "prefix_"))
}
