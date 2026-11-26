package connect

import (
	"encoding/json"
	"strconv"
	"testing"
)

// indxState is the state of an INDX as firmware 8.0 reports it: tool_count is
// eight for either build, and the heads that are not fitted are disabled.
func indxState(t *testing.T, fitted int) map[string]json.RawMessage {
	t.Helper()

	state := map[string]json.RawMessage{"/state": raw(t, "IDLE"), "/tool_count": raw(t, 8)}
	for n := range 8 {
		id := strconv.Itoa(n)
		state["/tools/"+id+"/enabled"] = raw(t, n < fitted)

		if n < fitted {
			state["/tools/"+id+"/type"] = raw(t, "FFF_PRINT_HEAD")
		}
	}

	return state
}

// A head the printer disabled is not fitted. The list leaves it out, so the
// slicer counts the heads that are there.
func TestADisabledHeadIsNotListed(t *testing.T) {
	got := PrinterJSON(Printer{ID: "p", Model: "COREONEINDX", Connected: true, State: indxState(t, 4)})

	tools, _ := got["tools"].(map[string]any)
	if len(tools) != 4 {
		t.Fatalf("an INDX with four heads lists %d tools: %v", len(tools), tools)
	}

	for _, id := range []string{"0", "1", "2", "3"} {
		if _, ok := tools[id]; !ok {
			t.Errorf("head %s is missing", id)
		}
	}
}

func TestINDXVariantFollowsTheFittedHeads(t *testing.T) {
	cases := []struct {
		fitted  int
		precise string
		tools   int
	}{
		{4, "COREONE_INDX4T", 4},
		{8, "COREONE_INDX8T", 8},
	}

	for _, c := range cases {
		got := PrinterJSON(Printer{ID: "p", Model: "COREONEINDX", Connected: true, State: indxState(t, c.fitted)})
		if got["legacy_printer_model"] != c.precise || got["tool_count"] != c.tools {
			t.Errorf("an INDX with %d heads fitted is %v with %v tools, want %s with %d",
				c.fitted, got["legacy_printer_model"], got["tool_count"], c.precise, c.tools)
		}
	}
}
