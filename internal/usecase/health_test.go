package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"govatars/internal/usecase"
)

type failProbe struct{}

func (failProbe) Health(context.Context) error { return errors.New("unavailable") }

type HealthSuite struct {
	suite.Suite
}

func (s *HealthSuite) TestStatus_AllSkipped() {
	h := usecase.NewHealth(nil, nil, nil)
	m := h.Status(context.Background())
	s.Equal("ok", m["status"])
}

func TestHealth_Degraded(t *testing.T) {
	h := usecase.NewHealth(failProbe{}, nil, nil)
	m := h.Status(context.Background())
	require.Equal(t, "degraded", m["status"])
	pg, ok := m["postgres"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, "down", pg["status"])
}

func TestHealthSuite(t *testing.T) {
	suite.Run(t, new(HealthSuite))
}
