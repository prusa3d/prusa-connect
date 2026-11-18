package mqtt

import "encoding/json"

// temperatureTopics are the printer-wide temperature paths and the frozen
// channel each one is published on. The chamber
// target has no channel in the frozen namespace.
var temperatureTopics = map[string]Topic{
	"/temperature/bed":        BedTemperature,
	"/temperature/bed_target": BedTarget,
	"/temperature/chamber":    ChamberTemperature,
}

// PublishTemperatureSample publishes one printer-wide temperature node on its
// channel, the way the nozzle topics are published: whole degrees, never below
// 0, and an empty payload for a removed node. A bed target of 0 is published,
// so a subscriber sees the heater go off. It does nothing for any other path or
// for a value that is not a number.
func (p *Publisher) PublishTemperatureSample(printerID, path string, value json.RawMessage) {
	topic, ok := temperatureTopics[path]
	if !ok {
		return
	}

	if payload, read := temperatureOrUnknown(value); read {
		p.offer(topic, topic.For(printerID), payload)
	}
}
