package opt

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/prusa3d/materials/internal/colour"
)

var (
	ErrNoManifest = errors.New("opt: zip has no data/manifest.yaml")
	ErrTooLarge   = errors.New("opt: zip is over the size limits")
	ErrEmpty      = errors.New("opt: zip holds no brands or materials")
)

type ref struct {
	Slug string `yaml:"slug"`
}

type yBrand struct {
	UUID string `yaml:"uuid"`
	Slug string `yaml:"slug"`
	Name string `yaml:"name"`
}

type yContainer struct {
	UUID         string   `yaml:"uuid"`
	Slug         string   `yaml:"slug"`
	Name         string   `yaml:"name"`
	Brand        ref      `yaml:"brand"`
	EmptyWeightG *float64 `yaml:"empty_weight"`
}

type yMaterial struct {
	UUID         string   `yaml:"uuid"`
	Slug         string   `yaml:"slug"`
	Brand        ref      `yaml:"brand"`
	Name         string   `yaml:"name"`
	Class        string   `yaml:"class"`
	Type         string   `yaml:"type"`
	Abbreviation string   `yaml:"abbreviation"`
	Tags         []string `yaml:"tags"`
	PrimaryColor struct {
		ColorRGBA string `yaml:"color_rgba"`
	} `yaml:"primary_color"`
	Properties struct {
		Density              *float64 `yaml:"density"`
		HardnessShoreD       *float64 `yaml:"hardness_shore_d"`
		TransmissionDistance *float64 `yaml:"transmission_distance"`
		MinPrintTemp         *int     `yaml:"min_print_temperature"`
		MaxPrintTemp         *int     `yaml:"max_print_temperature"`
		PreheatTemp          *int     `yaml:"preheat_temperature"`
		MinBedTemp           *int     `yaml:"min_bed_temperature"`
		MaxBedTemp           *int     `yaml:"max_bed_temperature"`
		ChamberTemp          *int     `yaml:"chamber_temperature"`
		MinChamberTemp       *int     `yaml:"min_chamber_temperature"`
		MaxChamberTemp       *int     `yaml:"max_chamber_temperature"`
		DryingTemp           *int     `yaml:"drying_temperature"`
		DryingTime           *int     `yaml:"drying_time"` // minutes
	} `yaml:"properties"`
}

type yPackage struct {
	UUID       string   `yaml:"uuid"`
	Slug       string   `yaml:"slug"`
	Class      string   `yaml:"class"`
	Material   ref      `yaml:"material"`
	Container  ref      `yaml:"container"`
	GTIN       string   `yaml:"gtin"` // a bare number in the YAML
	NetWeightG *float64 `yaml:"nominal_netto_full_weight"`
	// Diameter is in millimetres, as 1.75. A catalogue from before
	// 2026-11-23 writes micrometres, as 1750.
	Diameter *float64 `yaml:"filament_diameter"`
	LengthMM *int     `yaml:"nominal_full_length"`
}

// Read parses the catalogue ZIP. A YAML file that does not parse or has no
// uuid is skipped and counted. A ZIP over the limits, without a manifest,
// with an entry that cannot be read whole, or with no brand or material is
// refused: an entry that fails its checksum is a damaged download, and
// swapping it in would drop materials nobody chose to drop.
func Read(r io.ReaderAt, size int64, lim Limits) (Snapshot, error) {
	var s Snapshot
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return s, fmt.Errorf("opt: open zip: %w", err)
	}
	if len(zr.File) > lim.Entries {
		return s, ErrTooLarge
	}
	top := topOf(zr.File)
	if top == "" {
		return s, ErrNoManifest
	}

	var total int64
	for _, f := range zr.File {
		rest, ok := strings.CutPrefix(f.Name, top+"data/")
		if !ok || f.FileInfo().IsDir() || !strings.HasSuffix(rest, ".yaml") {
			continue
		}
		folder, _, _ := strings.Cut(rest, "/")
		if rest != "manifest.yaml" && folder != "brands" && folder != "materials" &&
			folder != "material-packages" && folder != "material-containers" {
			continue
		}
		b, readErr := readEntry(f, lim.File)
		if readErr != nil {
			return s, fmt.Errorf("opt: read %s: %w", f.Name, readErr)
		}
		total += int64(len(b))
		if int64(len(b)) > lim.File || total > lim.Total {
			return s, ErrTooLarge
		}
		if !s.add(rest, folder, b) {
			s.Skipped++
		}
	}
	s.deriveUUIDs()
	if len(s.Brands) == 0 || len(s.Materials) == 0 {
		return s, ErrEmpty
	}
	return s, nil
}

// topOf is the folder every file of the ZIP sits under ("repo-main/"), found
// by its data/manifest.yaml, or "" when there is none.
func topOf(files []*zip.File) string {
	for _, f := range files {
		if p, ok := strings.CutSuffix(f.Name, "/data/manifest.yaml"); ok && !strings.Contains(p, "/") {
			return p + "/"
		}
	}

	return ""
}

// readEntry reads one file, at most limit+1 bytes so the caller sees an
// oversize file.
func readEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}

	defer func() { _ = rc.Close() }()

	return io.ReadAll(io.LimitReader(rc, limit+1))
}

// add decodes one file into the snapshot and reports whether it was usable.
func (s *Snapshot) add(rest, folder string, b []byte) bool {
	switch {
	case rest == "manifest.yaml":
		var m struct {
			DataHash     string `yaml:"data_hash"`
			LastModified string `yaml:"last_modified"`
		}
		if yaml.Unmarshal(b, &m) != nil {
			return false
		}
		s.Meta = Meta{m.DataHash, m.LastModified}
	case folder == "brands":
		var y yBrand
		if yaml.Unmarshal(b, &y) != nil {
			return false
		}
		s.Brands = append(s.Brands, Brand(y))
	case folder == "material-containers":
		var y yContainer
		if yaml.Unmarshal(b, &y) != nil || y.UUID == "" {
			return false
		}
		s.Containers = append(s.Containers, Container{y.UUID, y.Slug, y.Brand.Slug, y.Name, y.EmptyWeightG})
	case folder == "materials":
		var y yMaterial
		if yaml.Unmarshal(b, &y) != nil {
			return false
		}
		p := y.Properties
		// The catalogue is a third party's: a colour that is not one reaches no page.
		if !colour.Valid(y.PrimaryColor.ColorRGBA) {
			y.PrimaryColor.ColorRGBA = ""
		}
		s.Materials = append(s.Materials, Material{
			UUID: y.UUID, Slug: y.Slug, BrandSlug: y.Brand.Slug, Name: y.Name, Class: y.Class,
			Type: y.Type, Abbreviation: y.Abbreviation, ColorRGBA: y.PrimaryColor.ColorRGBA,
			Density: p.Density, HardnessShoreD: p.HardnessShoreD, TransmissionDistance: p.TransmissionDistance,
			MinPrintTemp: p.MinPrintTemp, MaxPrintTemp: p.MaxPrintTemp, PreheatTemp: p.PreheatTemp,
			MinBedTemp: p.MinBedTemp, MaxBedTemp: p.MaxBedTemp, ChamberTemp: p.ChamberTemp,
			MinChamberTemp: p.MinChamberTemp, MaxChamberTemp: p.MaxChamberTemp,
			DryingTemp: p.DryingTemp, DryingTimeMin: p.DryingTime, Tags: y.Tags,
		})
	default:
		var y yPackage
		if yaml.Unmarshal(b, &y) != nil {
			return false
		}
		s.Packages = append(s.Packages, Package{y.UUID, y.Slug, y.Class, y.Material.Slug,
			y.Container.Slug, y.GTIN, y.NetWeightG, micrometres(y.Diameter), y.LengthMM})
	}
	return true
}

// Namespaces from the OpenPrintTag NFC data format, section 3.2.1.
var (
	brandNS    = uuid.MustParse("5269dfb7-1559-440a-85be-aba5f3eff2d2")
	materialNS = uuid.MustParse("616fc86d-7d99-4953-96c7-46d2836b9be9")
	packageNS  = uuid.MustParse("6f7d485e-db8d-4979-904e-a231cd6602b2")
)

func derive(ns uuid.UUID, parts ...[]byte) string {
	return uuid.NewSHA1(ns, bytesJoin(parts)).String()
}

func bytesJoin(parts [][]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// deriveUUIDs fills missing brand, material and package UUIDs the way the
// spec does. An entry whose UUID cannot be derived is dropped and counted.
// The brand's explicit uuid wins over a derived one.
func (s *Snapshot) deriveUUIDs() {
	brands := s.deriveBrands()
	s.derivePackages(brands, s.deriveMaterials(brands))
}

// deriveBrands returns the brand uuid by slug.
func (s *Snapshot) deriveBrands() map[string]uuid.UUID {
	brands := map[string]uuid.UUID{}
	keep := s.Brands[:0]
	for _, b := range s.Brands {
		if b.UUID == "" && b.Name != "" {
			b.UUID = derive(brandNS, []byte(b.Name))
		}
		id, err := uuid.Parse(b.UUID)
		if err != nil {
			s.Skipped++
			continue
		}
		brands[b.Slug] = id
		keep = append(keep, b)
	}
	s.Brands = keep

	return brands
}

// deriveMaterials returns the brand slug by material slug.
func (s *Snapshot) deriveMaterials(brands map[string]uuid.UUID) map[string]string {
	matBrand := map[string]string{}
	mats := s.Materials[:0]
	for _, m := range s.Materials {
		bid, ok := brands[m.BrandSlug]
		if m.UUID == "" && ok && m.Name != "" {
			m.UUID = derive(materialNS, bid[:], []byte(m.Name))
		}
		if m.UUID == "" {
			s.Skipped++
			continue
		}
		matBrand[m.Slug] = m.BrandSlug
		mats = append(mats, m)
	}
	s.Materials = mats

	return matBrand
}

func (s *Snapshot) derivePackages(brands map[string]uuid.UUID, matBrand map[string]string) {
	pkgs := s.Packages[:0]
	for _, p := range s.Packages {
		bid, ok := brands[matBrand[p.MaterialSlug]]
		if p.UUID == "" && ok && p.GTIN != "" {
			p.UUID = derive(packageNS, bid[:], []byte(p.GTIN))
		}
		if p.UUID == "" && p.Slug != "" {
			// Last fallback, an id derived from the slug.
			h := sha256.Sum256([]byte("material-package:" + p.Slug))
			x := hex.EncodeToString(h[:16])
			p.UUID = x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
		}
		if p.UUID == "" {
			s.Skipped++
			continue
		}
		pkgs = append(pkgs, p)
	}
	s.Packages = pkgs
}

// thinnestMicrometres splits the two units of a diameter: no filament is
// this many millimetres thick, nor this many micrometres thin.
// thickestMicrometres is a bound no filament reaches.
const (
	thinnestMicrometres = 100
	thickestMicrometres = 10_000
	micrometresPerMM    = 1000
)

// micrometres is a filament diameter in micrometres. A value under
// thinnestMicrometres is in millimetres. A diameter no filament has, as NaN,
// an infinity, 0 or a negative number, is unknown.
func micrometres(d *float64) *int {
	if d == nil {
		return nil
	}

	um := *d
	if um < thinnestMicrometres {
		um *= micrometresPerMM
	}

	// Written so that NaN, which fails every comparison, is unknown too.
	if !(um >= thinnestMicrometres && um <= thickestMicrometres) {
		return nil
	}

	n := int(math.Round(um))

	return &n
}
