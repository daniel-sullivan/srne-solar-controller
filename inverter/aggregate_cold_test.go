package inverter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAggregateAllNegativeTemperatures(t *testing.T) {
	units := []UnitSnapshot{
		{
			Battery: BatteryData{
				ControllerTemp: -12,
				BatteryTemp:    -15,
				BMSTemperature: -11,
			},
			Inverter: InverterData{
				HeatsinkTempA: -9,
				HeatsinkTempB: -7,
				HeatsinkTempC: -13,
				HeatsinkTempD: -6,
			},
		},
		{
			Battery: BatteryData{
				ControllerTemp: -8,
				BatteryTemp:    -10,
				BMSTemperature: -14,
			},
			Inverter: InverterData{
				HeatsinkTempA: -5,
				HeatsinkTempB: -12,
				HeatsinkTempC: -4,
				HeatsinkTempD: -9,
			},
		},
	}

	result := aggregate(units)

	assert.Equal(t, -8.0, result.Battery.ControllerTemp)
	assert.Equal(t, -10.0, result.Battery.BatteryTemp)
	assert.Equal(t, -11.0, result.Battery.BMSTemperature)
	assert.Equal(t, -5.0, result.Inverter.HeatsinkTempA)
	assert.Equal(t, -7.0, result.Inverter.HeatsinkTempB)
	assert.Equal(t, -4.0, result.Inverter.HeatsinkTempC)
	assert.Equal(t, -6.0, result.Inverter.HeatsinkTempD)
}
