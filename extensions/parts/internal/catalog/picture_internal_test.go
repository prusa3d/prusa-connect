package catalog

import "testing"

func TestAPrintFileWithoutAPictureDoesNotStandForItsPart(t *testing.T) {
	t.Parallel()

	no, yes := false, true
	file := func(name string, thumbnail *bool) File {
		return File{Name: name, Storage: "local", Path: name, Thumbnail: thumbnail}
	}

	for name, test := range map[string]struct {
		files []File
		want  string
	}{
		"none recorded: asked for as before": {[]File{file("a.bgcode", nil)}, "a.bgcode"},
		"found one":                          {[]File{file("a.bgcode", &yes)}, "a.bgcode"},
		"found none":                         {[]File{file("a.bgcode", &no)}, ""},
		"the next print file with one":       {[]File{file("a.bgcode", &no), file("b.3mf", &yes)}, "b.3mf"},
		"an image beats it":                  {[]File{file("a.bgcode", &yes), file("p.png", nil)}, "p.png"},
	} {
		got := pictureOf("", &Version{Files: test.files})

		switch {
		case test.want == "" && got != nil:
			t.Errorf("%s: drew %+v", name, got)
		case test.want != "" && (got == nil || got.Path != test.want):
			t.Errorf("%s: drew %+v, want %s", name, got, test.want)
		}
	}
}
