package opt

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

var zipPath = flag.String("zip", "", "real OpenPrintTag zip for TestReadRealZip")

func zipOf(t *testing.T, files map[string]string) (*bytes.Reader, int64) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes()), int64(buf.Len())
}

const asa = `uuid: m1
slug: prusament-asa-signal-red
brand:
  slug: prusament
name: ASA Signal Red
class: FFF
type: ASA
abbreviation: ASA
tags:
  - abrasive
  - contains_carbon
primary_color:
  color_rgba: '#f55928ff'
properties:
  density: 1.07
  min_print_temperature: 260
  max_print_temperature: 270
  min_bed_temperature: 100
  max_bed_temperature: 110
  chamber_temperature: 45
  drying_temperature: 80
  drying_time: 240
`

func catalogue(top string) map[string]string {
	d := top + "/data/"
	return map[string]string{
		d + "manifest.yaml":                "data_hash: abc\nlast_modified: '2026-10-27T15:16:56Z'\n",
		d + "brands/prusament.yaml":        "uuid: 863c5f0c-04a5-50c9-b5a2-5a8e81d5821f\nslug: prusament\nname: Prusament\n",
		d + "materials/prusament/asa.yaml": asa,
		d + "material-packages/p/pkg.yaml": "uuid: p1\nslug: pkg\nclass: FFF\nmaterial:\n  slug: a\ncontainer:\n  slug: c\nnominal_netto_full_weight: 850\nfilament_diameter: 1750\ngtin: 8594173673106\n",
		d + "material-containers/c.yaml":   "uuid: c1\nslug: c\nname: C\nbrand:\n  slug: prusament\nempty_weight: 190\n",
		top + "/ui-editor/x.ts":            "x",
	}
}

func TestReadKeepsTheFourFoldersAndTheManifest(t *testing.T) {
	r, n := zipOf(t, catalogue("openprinttag-database-main"))
	s, err := Read(r, n, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Brands) != 1 || len(s.Materials) != 1 || len(s.Packages) != 1 || len(s.Containers) != 1 {
		t.Fatalf("counts: %+v", s)
	}
	if s.Meta.DataHash != "abc" || s.Skipped != 0 {
		t.Fatalf("meta/skipped: %+v", s)
	}
	if p := s.Packages[0]; p.GTIN != "8594173673106" || *p.NetWeightG != 850 || *p.DiameterUM != 1750 ||
		p.ContainerSlug != "c" {
		t.Fatalf("package: %+v", p)
	}
	if m := s.Materials[0]; fmt.Sprint(m.Tags) != "[abrasive contains_carbon]" {
		t.Fatalf("tags: %v", m.Tags)
	}
	if c := s.Containers[0]; c.BrandSlug != "prusament" || *c.EmptyWeightG != 190 {
		t.Fatalf("container: %+v", c)
	}
}

func TestReadFindsTheTopFolderByItsManifest(t *testing.T) {
	r, n := zipOf(t, catalogue("foo-main"))
	s, err := Read(r, n, DefaultLimits)
	if err != nil || len(s.Materials) != 1 {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestReadSkipsAndCountsABrokenFile(t *testing.T) {
	c := catalogue("t")
	c["t/data/materials/x/bad.yaml"] = "{"
	r, n := zipOf(t, c)
	s, err := Read(r, n, DefaultLimits)
	if err != nil || s.Skipped != 1 || len(s.Materials) != 1 {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestReadRefusesAnEntryThatFailsItsChecksum(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range catalogue("db-main") {
		f, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	damaged := bytes.Replace(buf.Bytes(), []byte("ASA Signal Red"), []byte("ASA Signal Rot"), 1)
	if _, err := Read(bytes.NewReader(damaged), int64(len(damaged)), DefaultLimits); !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("a damaged entry: want zip.ErrChecksum, got %v", err)
	}
}

func TestReadRefusesAZipWithoutManifest(t *testing.T) {
	r, n := zipOf(t, map[string]string{"t/data/brands/b.yaml": "uuid: b\n"})
	if _, err := Read(r, n, DefaultLimits); !errors.Is(err, ErrNoManifest) {
		t.Fatalf("got %v", err)
	}
}

func TestReadRefusesAZipOverTheLimits(t *testing.T) {
	r, n := zipOf(t, map[string]string{"a": "1", "b": "2", "c": "3"})
	if _, err := Read(r, n, Limits{Entries: 2, Total: 1 << 20, File: 1 << 20}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("entries: %v", err)
	}
	c := catalogue("t")
	c["t/data/materials/x/big.yaml"] = "uuid: u\nname: 0123456789\n"
	r, n = zipOf(t, c)
	if _, err := Read(r, n, Limits{Entries: 100, Total: 1 << 20, File: 10}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("file: %v", err)
	}
}

func TestReadRefusesAnEmptyCatalogue(t *testing.T) {
	r, n := zipOf(t, map[string]string{"t/data/manifest.yaml": "data_hash: x\n"})
	if _, err := Read(r, n, DefaultLimits); !errors.Is(err, ErrEmpty) {
		t.Fatalf("got %v", err)
	}
}

func TestReadMapsTheSampleMaterial(t *testing.T) {
	r, n := zipOf(t, catalogue("t"))
	s, _ := Read(r, n, DefaultLimits)
	m := s.Materials[0]
	if m.ColorRGBA != "#f55928ff" || m.BrandSlug != "prusament" || m.Type != "ASA" {
		t.Fatalf("%+v", m)
	}
	if *m.Density != 1.07 || *m.MinPrintTemp != 260 || *m.MaxPrintTemp != 270 ||
		*m.MinBedTemp != 100 || *m.MaxBedTemp != 110 || *m.ChamberTemp != 45 ||
		*m.DryingTemp != 80 || *m.DryingTimeMin != 240 {
		t.Fatalf("%+v", m)
	}
	if m.PreheatTemp != nil || m.HardnessShoreD != nil {
		t.Fatalf("absent fields must stay nil: %+v", m)
	}
}

func TestReadDropsAColourThatIsNotOne(t *testing.T) {
	files := catalogue("t")
	files["t/data/materials/prusament/asa.yaml"] = strings.Replace(asa, "'#f55928ff'",
		"'#fff; background-image: url(https://evil.example/x)'", 1)
	r, n := zipOf(t, files)
	s, err := Read(r, n, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if m := s.Materials[0]; m.ColorRGBA != "" || m.Type != "ASA" {
		t.Fatalf("%+v", m)
	}
}

func TestDerivedUUIDsMatchTheSpecExample(t *testing.T) {
	d := "t/data/"
	r, n := zipOf(t, map[string]string{
		d + "manifest.yaml":            "data_hash: x\n",
		d + "brands/p.yaml":            "slug: prusament\nname: Prusament\n",
		d + "materials/p/g.yaml":       "slug: g\nname: PLA Prusa Galaxy Black\nbrand:\n  slug: prusament\n",
		d + "material-packages/a.yaml": "slug: a\nmaterial:\n  slug: g\ngtin: 1234\n",
		d + "material-packages/b.yaml": "slug: addnorth-adamant-s1-white-1kg\nmaterial:\n  slug: g\n",
		d + "material-packages/c.yaml": "material:\n  slug: g\n",
	})
	s, err := Read(r, n, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	// Values from the spec's algorithm (uuid.uuid5), computed in Python.
	if s.Brands[0].UUID != "ae5ff34e-298e-50c9-8f77-92a97fb30b09" ||
		s.Materials[0].UUID != "1aaca54a-431f-5601-adf5-85dd018f487f" {
		t.Fatalf("%+v %+v", s.Brands, s.Materials)
	}
	got := map[string]string{}
	for _, p := range s.Packages {
		got[p.Slug] = p.UUID
	}
	if len(got) != 2 || got["a"] != "7ed3ce83-764d-56de-bdcd-dc5226a0efd1" {
		t.Fatalf("package with gtin: %+v", s.Packages)
	}
	// sha256("material-package:<slug>") first 32 hex
	if got["addnorth-adamant-s1-white-1kg"] != "362d6a09-b4f8-055b-d6bb-fd1658eb797e" {
		t.Fatalf("slug hash: %+v", got)
	}
	if s.Skipped != 1 { // package c has no uuid, gtin or slug
		t.Fatalf("skipped %d", s.Skipped)
	}
}

// Opt-in: go test ./internal/opt/ -run Real -args -zip=/path/opt.zip
func TestReadRealZip(t *testing.T) {
	if *zipPath == "" {
		t.Skip("no -zip flag")
	}
	f, err := os.Open(*zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	s, err := Read(f, st.Size(), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("brands=%d materials=%d packages=%d containers=%d skipped=%d meta=%+v",
		len(s.Brands), len(s.Materials), len(s.Packages), len(s.Containers), s.Skipped, s.Meta)
}

func TestReadTakesAPackageDiameterInMillimetresOrMicrometres(t *testing.T) {
	for in, want := range map[string]int{"1.75": 1750, "2.85": 2850, "3": 3000, "1750": 1750} {
		files := catalogue("openprinttag-database-main")
		files["openprinttag-database-main/data/material-packages/p/pkg.yaml"] = "uuid: p1\nslug: pkg\nclass: FFF\n" +
			"material:\n  slug: a\nfilament_diameter: " + in + "\n"
		r, n := zipOf(t, files)
		s, err := Read(r, n, DefaultLimits)
		if err != nil {
			t.Fatal(err)
		}
		if p := s.Packages[0]; p.DiameterUM == nil || *p.DiameterUM != want {
			t.Errorf("filament_diameter %s: diameter %v µm, want %d", in, deref(p.DiameterUM), want)
		}
	}
}

// A diameter no filament has is unknown, and the package is still read.
func TestReadLeavesADiameterNoFilamentHasUnknown(t *testing.T) {
	for _, in := range []string{".nan", ".inf", "-.inf", "1e30", "0", "-1.75", "0.05", "50000"} {
		files := catalogue("openprinttag-database-main")
		files["openprinttag-database-main/data/material-packages/p/pkg.yaml"] = "uuid: p1\nslug: pkg\nclass: FFF\n" +
			"material:\n  slug: a\nfilament_diameter: " + in + "\n"
		r, n := zipOf(t, files)
		s, err := Read(r, n, DefaultLimits)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Packages) != 1 || s.Packages[0].DiameterUM != nil {
			t.Errorf("filament_diameter %s: packages %+v, want one with no diameter", in, s.Packages)
		}
	}
}

func deref(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}
