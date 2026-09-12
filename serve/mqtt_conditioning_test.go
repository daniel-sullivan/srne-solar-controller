package serve

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type conditioningCommandStub struct {
	status ConditioningServiceStatus
	starts int
	stops  int
	err    error
}

func (s *conditioningCommandStub) Start(context.Context) error {
	s.starts++
	return s.err
}

func (s *conditioningCommandStub) Stop(context.Context) error {
	s.stops++
	return s.err
}

func (s *conditioningCommandStub) Status() ConditioningServiceStatus { return s.status }

func TestMQTTConditioningRequiresLiveExplicitCommand(t *testing.T) {
	controller := &conditioningCommandStub{status: ConditioningServiceStatus{Enabled: true}}
	pub := &MQTTPublisher{}
	pub.SetConditioning(controller)

	require.NoError(t, pub.handleConditioningCommand("ON", true))
	require.NoError(t, pub.handleConditioningCommand("OFF", true))
	require.Zero(t, controller.starts)
	require.Zero(t, controller.stops)

	require.NoError(t, pub.handleConditioningCommand("ON", false))
	require.NoError(t, pub.handleConditioningCommand("OFF", false))
	require.Equal(t, 1, controller.starts)
	require.Equal(t, 1, controller.stops)
	require.Error(t, pub.handleConditioningCommand("unexpected", false))
	require.Equal(t, 1, controller.starts)
	require.Equal(t, 1, controller.stops)
}

func TestMQTTConditioningDisabledAndActuationError(t *testing.T) {
	pub := &MQTTPublisher{}
	require.Error(t, pub.handleConditioningCommand("ON", false))
	controller := &conditioningCommandStub{}
	pub.SetConditioning(controller)
	require.Error(t, pub.handleConditioningCommand("ON", false))
	require.Zero(t, controller.starts)

	controller.status.Enabled = true
	controller.err = errors.New("inverter refused write")
	require.ErrorContains(t, pub.handleConditioningCommand("ON", false), "inverter refused write")
	require.Equal(t, 1, controller.starts)
}
