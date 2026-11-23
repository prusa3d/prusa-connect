package resource

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/kernel/ref"
)

// FileCamera is one camera an older build declared in a file. No file
// declares a camera any more: the host moves these into its database once
// (ADR-20261116-g5x).
type FileCamera struct {
	Name   string
	Type   string
	Config json.RawMessage
	Source Source
}

// FileCameras is the cameras both files declare, by name. A camera both
// declare is the operator's file's, as it was. A block that does not read is
// a failure, and the rest are still answered.
func FileCameras(paths Paths) ([]FileCamera, []Failure, error) {
	read, err := readInputs(paths)
	if err != nil {
		return nil, nil, err
	}

	overlay, err := parseOverlay(read.overlay)
	if err != nil {
		return nil, nil, err
	}

	byName := map[string]FileCamera{}

	for _, entry := range overlay.Instances {
		if entry.Kind == ref.KindCamera.String() {
			byName[entry.Name] = FileCamera{
				Name: entry.Name, Type: entry.Type, Config: entry.Config, Source: SourceOverlay,
			}
		}
	}

	var doc struct {
		Resources map[string]map[string]yaml.Node `yaml:"resources"`
	}

	if err = yaml.Unmarshal(read.file, &doc); err != nil {
		return nil, nil, fmt.Errorf("%w: parse the resources block: %w", ErrConfigInvalid, err)
	}

	var failures []Failure

	for name, node := range doc.Resources[ref.KindCamera.String()] {
		spec, specErr := fileSpec(node)
		if specErr != nil {
			key := "resources." + ref.KindCamera.String() + "." + name
			failures = append(
				failures,
				Failure{Key: key, Err: fmt.Errorf("%w: %s: %w", ErrConfigInvalid, key, specErr)},
			)

			continue
		}

		byName[name] = FileCamera{Name: name, Type: spec.Type, Config: spec.Config, Source: SourceFile}
	}

	cameras := slices.SortedFunc(
		maps.Values(byName),
		func(a, b FileCamera) int { return strings.Compare(a.Name, b.Name) },
	)

	return cameras, failures, nil
}

// DropOverlayCameras takes the cameras out of the overlay, once the host holds
// them.
func DropOverlayCameras(paths Paths) error {
	overlayWriting.Lock()
	defer overlayWriting.Unlock()

	read, err := readInputs(paths)
	if err != nil {
		return err
	}

	overlay, err := parseOverlay(read.overlay)
	if err != nil {
		return err
	}

	if !slices.ContainsFunc(overlay.Instances, isCameraEntry) {
		return nil
	}

	overlay.Instances = slices.DeleteFunc(overlay.Instances, isCameraEntry)
	maps.DeleteFunc(overlay.Identities, func(path string, _ overlayIdentity) bool {
		return strings.HasPrefix(path, ref.KindCamera.String()+"/")
	})

	return writeOverlayFile(paths.Overlay, overlay)
}

func isCameraEntry(entry overlayEntry) bool { return entry.Kind == ref.KindCamera.String() }
