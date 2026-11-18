package mqtt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prusa3d/prusa-connect/sdk/eventsapi"
)

// The bed and chamber paths go to their frozen
// channels, from a partial sample and from a whole tree alike.
func TestTheBedAndChamberPathsArePublishedOnTheirChannels(t *testing.T) {
	t.Parallel()

	for name, whole := range map[string]bool{"partial": false, "whole": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := &Processor{Publisher: queuedPublisher()}

			p.SamplesObserved(eventsapi.Samples{DeviceID: "abc", Whole: whole, Nodes: []eventsapi.Sample{
				{Path: "/temperature/bed", Value: json.RawMessage(`59.6`)},
				{Path: "/temperature/bed_target", Value: json.RawMessage(`60`)},
				{Path: "/temperature/chamber", Value: json.RawMessage(`31.2`)},
				{Path: "/temperature/chamber_target", Value: json.RawMessage(`40`)},
			}})

			got := map[string]string{}
			for topic, msg := range queued(p.Publisher) {
				got[topic] = msg.payload
			}

			assert.Equal(t, map[string]string{
				"v1/devices/printers/abc/data/temp/heatbed/current": "60",
				"v1/devices/printers/abc/data/temp/heatbed/target":  "60",
				"v1/devices/printers/abc/data/temp/chamber/current": "31",
			}, got)
		})
	}
}

// A removed node publishes an empty payload, as the nozzle topics do, so a
// live subscriber stops showing a reading the printer no longer reports. A
// value that is not a number publishes nothing.
func TestARemovedTemperatureIsPublishedEmpty(t *testing.T) {
	t.Parallel()

	p := &Processor{Publisher: queuedPublisher()}

	p.SamplesObserved(eventsapi.Samples{DeviceID: "abc", Nodes: []eventsapi.Sample{
		{Path: "/temperature/chamber", Value: json.RawMessage(`null`)},
		{Path: "/temperature/bed", Value: json.RawMessage(`"hot"`)},
	}})

	got := map[string]string{}
	for topic, msg := range queued(p.Publisher) {
		got[topic] = msg.payload
	}

	assert.Equal(t, map[string]string{
		"v1/devices/printers/abc/data/temp/chamber/current": "",
	}, got)
}

// A bed target of 0 is published, as the nozzle target is, so a subscriber
// sees the heater go off and then sees it come back on to the same target.
func TestABedTargetThatGoesOffAndBackIsPublishedEachTime(t *testing.T) {
	t.Parallel()

	p := &Processor{Publisher: queuedPublisher()}

	var sent []string

	for _, target := range []string{"60", "0", "60"} {
		p.SamplesObserved(eventsapi.Samples{DeviceID: "abc", Nodes: []eventsapi.Sample{
			{Path: "/temperature/bed_target", Value: json.RawMessage(target)},
		}})

		for _, msg := range queued(p.Publisher) {
			sent = append(sent, msg.payload)
		}
	}

	assert.Equal(t, []string{"60", "0", "60"}, sent)
}
