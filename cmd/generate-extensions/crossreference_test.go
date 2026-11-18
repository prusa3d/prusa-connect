package main

import (
	"errors"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// TestEveryShippedSinkPublishesTheAddressesItsWireCarries is the destination
// half of the descriptor's two publication fields. A sink puts messages on a
// transport of its own, and what a subscriber of that transport matches on is a
// topic, a routing key or a payload discriminator, not a type in the host's
// event vocabulary.
//
// The addresses are written out rather than read from the adapter, because a
// gate that asks the code what it produces agrees with every answer including
// the wrong one. That is how the published MQTT list came to be wrong on four
// of five entries: it was a second copy of topics.go, typed by hand.
func TestEveryShippedSinkPublishesTheAddressesItsWireCarries(t *testing.T) {
	t.Parallel()

	// The kinds the three JSON sinks put in the "kind" member of every body,
	// which amqp also suffixes onto its routing key prefix.
	kinds := []string{
		"printer.state_changed",
		"printer.labels_changed",
		"printer.deleted",
		"job.updated",
		"label.deleted",
		"fact.observed",
	}

	for _, testCase := range []struct {
		entry string
		want  []string
	}{
		{entry: "event-sink core/amqp", want: kinds},
		{entry: "event-sink core/kafka", want: kinds},
		{entry: "event-sink core/webhook", want: kinds},
		{entry: "event-sink core/mqtt", want: []string{
			"v1/server/online",
			"v1/devices/printers/{printerId}/data/state",
			"v1/devices/printers/{printerId}/data/online",
			"v1/devices/printers/{printerId}/data/current-job",
			"v1/devices/printers/{printerId}/jobs/{jobId}/data/state",
			"v1/devices/printers/{printerId}/data/tools/{toolId}/active",
			"v1/devices/printers/{printerId}/data/tools/{toolId}/material",
			"v1/devices/printers/{printerId}/data/tools/{toolId}/temp/nozzle/current",
			"v1/devices/printers/{printerId}/data/tools/{toolId}/temp/nozzle/target",
			"v1/devices/printers/{printerId}/data/temp/heatbed/current",
			"v1/devices/printers/{printerId}/data/temp/heatbed/target",
			"v1/devices/printers/{printerId}/data/temp/chamber/current",
			"v1/events/{origin}/{eventType}",
		}},
	} {
		t.Run(testCase.entry, func(t *testing.T) {
			t.Parallel()

			descriptor, ok := registeredEntry(testCase.entry)
			if !ok {
				t.Fatalf("%s is not registered, so this gate is comparing against nothing", testCase.entry)
			}

			doc := descriptor.Doc()

			if len(doc.Emits) != 0 {
				t.Errorf("%s declares %d emitted event types, and a sink puts nothing on the host's "+
					"event bus", testCase.entry, len(doc.Emits))
			}

			addresses := make([]string, 0, len(doc.Publishes))
			for _, destination := range doc.Publishes {
				addresses = append(addresses, destination.Address)
			}

			if len(addresses) != len(testCase.want) {
				t.Fatalf("%s publishes to %v, want %v", testCase.entry, addresses, testCase.want)
			}

			for i, want := range testCase.want {
				if addresses[i] != want {
					t.Errorf("%s publishes to %q at position %d, want %q",
						testCase.entry, addresses[i], i, want)
				}
			}
		})
	}
}

// TestAnEmittedEventTypeIsRefusedWhileTheShippedSinksStillRegister is the trap
// this gate exists to avoid. A test that only asserts a bad event type is
// refused passes against a Declare that refuses everything, so the two halves
// are one test: the four shipped sinks are in the catalogue that this command
// publishes from, and a descriptor naming an event type sdk/event refuses is
// not.
func TestAnEmittedEventTypeIsRefusedWhileTheShippedSinksStillRegister(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{
		"event-sink core/amqp", "event-sink core/kafka",
		"event-sink core/mqtt", "event-sink core/webhook",
	} {
		if _, ok := registeredEntry(entry); !ok {
			t.Fatalf("%s did not register, so the refusal below refuses everything", entry)
		}
	}

	doc := extension.Doc{
		Summary:     "Posts events to one endpoint.",
		Description: "The longer paragraph an author reads before choosing this entry.",
		Version:     "1.0.0",
		Contract:    "1.0.0",
		Stability:   extension.StabilityStable,
		Emits: []extension.Emission{
			{Type: "NOT even reverse dns", Version: "1.0.0", Summary: "something happened."},
		},
	}

	if _, err := extension.Declare(extension.KindEventSink, "1.0.0", "forged", doc); !errors.Is(
		err, extension.ErrInvalidText,
	) {
		t.Fatalf("Declare emitting %q = %v, want ErrInvalidText", doc.Emits[0].Type, err)
	}

	doc.Emits[0].Type = "com.prusa.connect.device.arrived"
	if _, err := extension.Declare(extension.KindEventSink, "1.0.0", "forged", doc); err != nil {
		t.Fatalf("Declare emitting a type the vocabulary carries: %v", err)
	}
}

// registeredEntry finds one entry by the string the reference and every error
// message name it, out of the catalogue this command's blank imports fill.
func registeredEntry(name string) (extension.Descriptor, bool) {
	for _, entry := range extension.Global().All() {
		if entry.String() == name {
			return entry, true
		}
	}

	return extension.Descriptor{}, false
}
