package otelpkg_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"govatars/internal/pkg/config"
	"govatars/internal/pkg/otelpkg"
)

func TestNewResource_serviceAttributes(t *testing.T) {
	t.Parallel()

	res, err := otelpkg.NewResource(context.Background(), config.OTELResource{
		ServiceName:    "govatars_test",
		ServiceVersion: "0.0.1",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
}
