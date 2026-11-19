package moonraker

import (
	"encoding/json"
	"strings"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The Klipper sections that become readings, by the prefix of their object
// name. A heater_generic has a target. A temperature_sensor has none.
const (
	heaterGenericPrefix     = "heater_generic "
	temperatureSensorPrefix = "temperature_sensor "
)

// temperaturePrefix is where the value of each sensor lands, beside the bed.
const temperaturePrefix = "/temperature/"

// sensor is one heater_generic or temperature_sensor section and the reading
// that describes it.
type sensor struct {
	object  string
	reading connectorapi.Reading
}

// bedReading describes the heated bed.
func bedReading() connectorapi.Reading {
	return connectorapi.Reading{
		ID: "bed", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: bedPath, TargetPath: bedTargetPath, Role: connectorapi.RoleBed,
	}
}

// sensorsOf is every sensor section the machine lists, in list order. It skips
// a section whose id or paths another reading already holds.
func sensorsOf(objects []string) []sensor {
	taken := map[string]bool{"bed": true, bedPath: true, bedTargetPath: true}

	var out []sensor

	for _, object := range objects {
		name, heater := strings.CutPrefix(object, heaterGenericPrefix)
		if !heater {
			var found bool
			if name, found = strings.CutPrefix(object, temperatureSensorPrefix); !found {
				continue
			}
		}

		reading := sensorReading(name, heater)
		if reading.Problems() != nil || taken[reading.ID] || taken[reading.ValuePath] || taken[reading.TargetPath] {
			continue
		}

		taken[reading.ID], taken[reading.ValuePath] = true, true
		if reading.TargetPath != "" {
			taken[reading.TargetPath] = true
		}

		out = append(out, sensor{object: object, reading: reading})
	}

	return out
}

// sensorReading describes one section by the name its owner gave it.
func sensorReading(name string, heater bool) connectorapi.Reading {
	id := readingID(name)
	reading := connectorapi.Reading{
		ID: id, Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: temperaturePrefix + id,
	}

	if heater {
		reading.TargetPath = temperaturePrefix + id + "_target"
	}

	if len(name) <= connectorapi.MaxReadingNameLen {
		reading.Name = name
	}

	if name == "chamber" {
		reading.Role = connectorapi.RoleChamber
	}

	return reading
}

// readingID turns a section name into a reading id: lower case, any other
// character an underscore, and a leading s_ when it does not start with a
// letter.
func readingID(name string) string {
	var id strings.Builder

	for _, c := range []byte(strings.ToLower(name)) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			id.WriteByte(c)
		} else {
			id.WriteByte('_')
		}
	}

	out := id.String()
	if out == "" || out[0] < 'a' || out[0] > 'z' {
		out = "s_" + out
	}

	return out[:min(len(out), maxReadingID)]
}

// maxReadingID is the longest id a reading may have.
const maxReadingID = 64

// readingPatches describes the bed, when the machine has one, and every sensor.
func readingPatches(objects []string, sensors []sensor) []connectorapi.StatePatch {
	var out []connectorapi.StatePatch

	for _, object := range objects {
		if object == objectHeaterBed {
			out = append(out, bedReading().Patches()...)
		}
	}

	for _, s := range sensors {
		out = append(out, s.reading.Patches()...)
	}

	return out
}

// sensorObjects is the object name of each sensor, for the subscription.
func sensorObjects(sensors []sensor) []string {
	out := make([]string, 0, len(sensors))
	for _, s := range sensors {
		out = append(out, s.object)
	}

	return out
}

// appendSensors writes the value of each sensor, and the target of a heater.
func appendSensors(
	out []connectorapi.StatePatch, sensors []sensor, objects map[string]map[string]json.RawMessage,
) []connectorapi.StatePatch {
	for _, s := range sensors {
		fields := objects[s.object]
		out = appendVerbatim(out, fields["temperature"], s.reading.ValuePath)

		if s.reading.TargetPath != "" {
			out = appendVerbatim(out, fields["target"], s.reading.TargetPath)
		}
	}

	return out
}
