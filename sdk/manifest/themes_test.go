package manifest_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/prusa3d/prusa-connect/sdk/manifest"
)

// withThemes is the smallest ui block that carries the themes list given.
func withThemes(themes string) string {
	return withUI(`{"entry":"ui/index.js","library":">=0.3.0 <1.0.0","themes":` + themes + `}`)
}

func TestAThemeListIsRead(t *testing.T) {
	t.Parallel()

	document := withThemes(`[
	  {"id":"ocean","title":"Ocean","stylesheet":"ui/ocean.css"},
	  {"id":"paper","title":"Paper","stylesheet":"ui/themes/paper.css"}
	]`)

	held, err := manifest.Parse("module.yaml", []byte(document), decode)
	if err != nil {
		t.Fatalf("a valid themes list was refused: %v", err)
	}

	want := []manifest.Theme{
		{ID: "ocean", Title: "Ocean", Stylesheet: "ui/ocean.css"},
		{ID: "paper", Title: "Paper", Stylesheet: "ui/themes/paper.css"},
	}
	if !reflect.DeepEqual(held.UI.Themes, want) {
		t.Errorf("the themes read as %+v, want %+v", held.UI.Themes, want)
	}
}

func TestABlockWithNoThemesOffersNone(t *testing.T) {
	t.Parallel()

	held, err := manifest.Parse("module.yaml", []byte(withUI(wholeUI)), decode)
	if err != nil {
		t.Fatalf("the whole block was refused: %v", err)
	}

	if held.UI.Themes != nil {
		t.Errorf("a block with no themes read as %+v", held.UI.Themes)
	}
}

func TestEveryThemeKeyIsValidated(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		themes  string
		refuses string
	}{
		"no id":          {`[{"title":"Ocean","stylesheet":"ui/o.css"}]`, "id is empty"},
		"id not a label": {`[{"id":"Ocean","title":"Ocean","stylesheet":"ui/o.css"}]`, "one label"},
		"no title":       {`[{"id":"ocean","stylesheet":"ui/o.css"}]`, "title is empty"},
		"no stylesheet":  {`[{"id":"ocean","title":"Ocean"}]`, "stylesheet is empty"},
		"climbs out":     {`[{"id":"ocean","title":"Ocean","stylesheet":"../o.css"}]`, "climb out of it"},
		"absolute":       {`[{"id":"ocean","title":"Ocean","stylesheet":"/ui/o.css"}]`, "starts with no slash"},
		"not css":        {`[{"id":"ocean","title":"Ocean","stylesheet":"ui/o.js"}]`, "ends in .css"},
		"unknown key":    {`[{"id":"ocean","title":"Ocean","stylesheet":"ui/o.css","url":"x"}]`, "url"},
		"id written twice": {
			`[{"id":"ocean","title":"A","stylesheet":"ui/a.css"},{"id":"ocean","title":"B","stylesheet":"ui/b.css"}]`,
			"names ocean twice",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			refuse(t, withThemes(testCase.themes), testCase.refuses)
		})
	}
}

func TestTheThemeListIsBounded(t *testing.T) {
	t.Parallel()

	entries := make([]string, 0, manifest.MaxThemes+1)
	for index := range manifest.MaxThemes + 1 {
		entries = append(entries,
			fmt.Sprintf(`{"id":"t%d","title":"T","stylesheet":"ui/t%d.css"}`, index, index))
	}

	refuse(t, withThemes("["+strings.Join(entries, ",")+"]"), "the limit is")
}

func TestABundleThatOnlyOffersThemesNeedsNoEntry(t *testing.T) {
	t.Parallel()

	document := withUI(`{"library":">=0.3.0 <1.0.0",` +
		`"themes":[{"id":"ocean","title":"Ocean","stylesheet":"ocean.css"}]}`)

	held, err := manifest.Parse("module.yaml", []byte(document), decode)
	if err != nil {
		t.Fatalf("a bundle of themes alone was refused: %v", err)
	}

	if held.UI.Entry != "" || len(held.UI.Themes) != 1 {
		t.Errorf("the block read as entry %q and themes %+v", held.UI.Entry, held.UI.Themes)
	}
}

func TestABundleWithThemesAndPagesStillNeedsAnEntry(t *testing.T) {
	t.Parallel()

	refuse(t, withUI(`{"library":">=0.3.0 <1.0.0",`+
		`"themes":[{"id":"ocean","title":"Ocean","stylesheet":"ocean.css"}],`+
		`"contributes":{"pages":[{"path":"spools","title":"Spools"}]}}`), "entry is empty")
}
