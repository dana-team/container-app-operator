package utils_test

import (
	"testing"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/utils"

	"github.com/stretchr/testify/require"
)

func TestManagedResourceLabels(t *testing.T) {
	const cappName = "my-capp"
	labels := utils.ManagedResourceLabels(cappName)
	require.Equal(t, cappName, labels[utils.CappResourceKey])
	require.Equal(t, utils.CappKey, labels[utils.ManagedByLabelKey])
}

func TestExcludeKeysWithPrefix(t *testing.T) {
	object := map[string]string{
		"prefix_key1": "value1",
		"key2":        "value2",
		"prefix_key3": "value3",
	}
	require.Equal(t, map[string]string{"key2": "value2"}, utils.ExcludeKeysWithPrefix(object, "prefix_"))
}
