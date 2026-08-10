package cappmeta_test

import (
	"testing"

	"github.com/dana-team/container-app-operator/internal/kinds/capp/cappmeta"

	"github.com/stretchr/testify/require"
)

func TestManagedResourceLabels(t *testing.T) {
	const cappName = "my-capp"
	labels := cappmeta.ManagedResourceLabels(cappName)
	require.Equal(t, cappName, labels[cappmeta.CappResourceKey])
	require.Equal(t, cappmeta.CappKey, labels[cappmeta.ManagedByLabelKey])
}
