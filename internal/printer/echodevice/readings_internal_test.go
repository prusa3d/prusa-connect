package echodevice

import (
	"encoding/json"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/connectorapi"
)

func TestTheFactsDescribeTheBedThisMachineReports(t *testing.T) {
	t.Parallel()

	tree := map[string]json.RawMessage{}
	for _, patch := range (&Device{}).facts() {
		tree[patch.Path] = patch.Value
	}

	readings := connectorapi.ReadReadings(tree)
	if len(readings) != 1 || readings[0] != bedReading() {
		t.Fatalf("readings %+v, want the bed", readings)
	}

	if readings[0].ValuePath != bedPath || readings[0].TargetPath != bedTargetPath {
		t.Errorf("the bed is described at %s and %s", readings[0].ValuePath, readings[0].TargetPath)
	}

	if problems := readings[0].Problems(); len(problems) > 0 {
		t.Errorf("the bed: %v", problems)
	}
}
