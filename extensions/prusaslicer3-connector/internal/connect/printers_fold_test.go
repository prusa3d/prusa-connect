package connect

import "testing"

// The firmware reports the INDX models by the ids of its own model table,
// COREONEINDX and COREONEL-INDX, with no underscore. The printer list must
// still name the cloud's model, or PrusaSlicer finds no printer preset.
func TestFirmwareSpellingOfINDX(t *testing.T) {
	cases := []struct {
		reported string
		tools    int
		precise  string
		model    string
	}{
		{"COREONEINDX", 8, "COREONE_INDX8T", "COREONE_INDX"},
		{"COREONEINDX", 4, "COREONE_INDX4T", "COREONE_INDX"},
		{"COREONEL-INDX", 8, "COREONEL_INDX8T", "COREONEL_INDX"},
		{"CoreOne_INDX", 8, "COREONE_INDX8T", "COREONE_INDX"},
		{"MK4S", 1, "MK4S", "MK4S"},
		{"MK3.9", 1, "MK3.9", "MK3.9"},
	}

	for _, c := range cases {
		precise := preciseModel(c.reported, c.tools, false)
		if precise != c.precise {
			t.Errorf("%s with %d tools is %q, want %q", c.reported, c.tools, precise, c.precise)
		}

		if row := legacyModels[precise]; row.model != c.model {
			t.Errorf("%s reports model %q, want %q", c.reported, row.model, c.model)
		}
	}
}
