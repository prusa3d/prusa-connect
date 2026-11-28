package manifest

import (
	"fmt"
	"slices"
	"strings"
)

// Theme is one stylesheet a bundle offers as a look for the whole app. The
// app loads it after its own styles when a person picks it, so it may
// override anything the app draws.
type Theme struct {
	// ID is the theme's name within the module, one label.
	ID string
	// Title is what the theme picker reads.
	Title string
	// Stylesheet is the theme's CSS file, relative to the bundle root, such as
	// ui/theme.css. The host serves it under the module's namespace.
	Stylesheet string
}

// MaxThemes is how many themes one bundle may offer.
const MaxThemes = 8

// stylesheetSuffix is what a theme's stylesheet must end in, so the host
// serves it as text/css.
const stylesheetSuffix = ".css"

//nolint:gochecknoglobals // a closed table read at parse, never written.
var themeKeys = []string{"id", "title", "stylesheet"}

// readThemes reads ui.themes. Each id is one label, each title is not empty,
// each stylesheet is a .css file inside the bundle, and no id is written twice.
func readThemes(block map[string]any) ([]Theme, error) {
	entries, err := nodeList(block, "ui", "themes")
	if err != nil || entries == nil {
		return nil, err
	}

	if len(entries) > MaxThemes {
		return nil, fmt.Errorf("%w: ui.themes names %d themes, and the limit is %d",
			ErrInvalid, len(entries), MaxThemes)
	}

	out := make([]Theme, 0, len(entries))

	for index, entry := range entries {
		where := fmt.Sprintf("ui.themes[%d]", index)

		if err = refuseUnknown(entry, where, themeKeys); err != nil {
			return nil, err
		}

		var held Theme

		if held.ID, err = label(entry, where, "id"); err != nil {
			return nil, err
		}

		if held.Title, err = title(entry, where); err != nil {
			return nil, err
		}

		if held.Stylesheet, err = requiredText(entry, where, "stylesheet"); err != nil {
			return nil, err
		}

		if err = insideBundle(where+".stylesheet", held.Stylesheet); err != nil {
			return nil, err
		}

		if !strings.HasSuffix(held.Stylesheet, stylesheetSuffix) {
			return nil, fmt.Errorf("%w: %s.stylesheet is %q, and a theme's stylesheet ends in %s",
				ErrInvalid, where, held.Stylesheet, stylesheetSuffix)
		}

		if slices.ContainsFunc(out, func(o Theme) bool { return o.ID == held.ID }) {
			return nil, fmt.Errorf("%w: ui.themes names %s twice", ErrInvalid, held.ID)
		}

		out = append(out, held)
	}

	return out, nil
}
