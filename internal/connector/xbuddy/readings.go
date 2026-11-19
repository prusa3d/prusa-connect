package xbuddy

import (
	"encoding/json"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

// The bed's reading and the target it is held to.
const (
	bedTempPath   = "/temperature/bed"
	bedTargetPath = "/temperature/bed_target"
)

// bedReading describes the bed every model reports.
func bedReading() connectorapi.Reading {
	return connectorapi.Reading{
		ID: "bed", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: bedTempPath, TargetPath: bedTargetPath, Role: connectorapi.RoleBed,
	}
}

// chamberReading describes the chamber of a CORE One, which has a target, or
// the enclosure of an XL, which has none.
func chamberReading(withTarget bool) connectorapi.Reading {
	reading := connectorapi.Reading{
		ID: "chamber", Kind: connectorapi.ReadingTemperature, Unit: connectorapi.UnitCelsius,
		ValuePath: chamberTempPath, Role: connectorapi.RoleChamber,
	}

	if withTarget {
		reading.TargetPath = chamberTargetPath
	}

	return reading
}

// readingPath reports whether a path is a member of a reading description. A
// description is written once, so a backlog must not drop it.
func readingPath(path string) bool {
	_, _, ok := connectorapi.SplitReadingPath(path)

	return ok
}

// describeBed describes the bed with the first frame that carries its reading.
// The host publishes every patch, so the description is not repeated.
func (s *Session) describeBed(fields map[string]json.RawMessage) {
	if _, present := fields["temp_bed"]; !present || s.bedDescribed {
		return
	}

	s.bedDescribed = true

	for _, patch := range bedReading().Patches() {
		s.report(patch)
	}
}

// describeChamber keeps the chamber's description in step with its reading:
// described while a reading stands, removed when it goes.
func (s *Session) describeChamber() {
	var next connectorapi.Reading
	if s.chamberReported {
		next = chamberReading(s.chamberTargetReported)
	}

	if next == s.chamberDescribed {
		return
	}

	patches := next.Patches()
	if next.ID == "" {
		patches = s.chamberDescribed.Removal()
	}

	s.chamberDescribed = next

	for _, patch := range patches {
		s.report(patch)
	}
}
