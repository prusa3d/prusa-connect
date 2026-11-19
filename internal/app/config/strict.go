package config

import (
	"fmt"
	"maps"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodesPerEntry is how many nodes one mapping entry occupies. A yaml.Node
// mapping holds its keys and values alternating in one slice, in file order,
// each with its line number.
const nodesPerEntry = 2

// nullTag is the tag the parser gives a key written with nothing under it. A
// block header with nothing under it means the defaults.
const nullTag = "!!null"

// yamlNodeType is a raw block this package passes on without reading: a
// connector's settings belong to its driver. yamlUnmarshaler is a type that
// decodes itself. Neither can be held to a list of field names.
//
//nolint:gochecknoglobals // two reflect types resolved once and only read.
var (
	yamlNodeType    = reflect.TypeFor[yaml.Node]()
	yamlUnmarshaler = reflect.TypeFor[yaml.Unmarshaler]()
)

// retiredKeys are the keys this build does not read, each with the sentence
// that names the key that replaced it, or that no key does.
//
// A retired key is refused like any other key this build does not read, and
// the message adds the replacement, so the operator can fix the file with a
// one-line edit. The build tool refuses `server.api` the same way; see
// internal/app/config/build/tags.go.
//
//nolint:gochecknoglobals // the inventory of retired keys, read at parse time and never written.
var retiredKeys = map[string]string{
	"core.printer": "The device-facing listener belongs to the connector that serves those " +
		"devices. Write it under core.connectors.<name>.config.listener, which takes addr, tls " +
		"and handshake_timeout",
	"core.compat": "The GraphQL module is switched by its own row under resources: " +
		"resources.sink.graphql.enabled",
	// The three retired top-level blocks. They are keyed by a bare name, and
	// every other key in this map starts with a block name, so the two cannot
	// collide.
	"mqtt": "This build serves no API over MQTT. To send the printers' state to a broker you " +
		"run, declare a resources.sink row of type mqtt with the broker's url",
	"observability": "The metrics client and the exposition at /metrics are core.observability.enabled",
	"fractal": "A link to a parent is declared like every other resource instance: " +
		"resources.link.<name>.enabled, with the address under its config",
	"core.frontend.skin": "The web app has one appearance and no key chooses it. Delete the line",
}

// decodeStrictly fills target from one block of the operator's file and refuses
// any key target has no field for, naming the key path and the line of the file
// it is written on.
//
// It walks the parsed nodes and does not use yaml.Decoder.KnownFields.
// KnownFields counts line numbers against the reader it was handed, and the
// only reader for one block of a document is a fragment re-encoded from that
// block. A line counted from the top of a fragment is the wrong line of the
// operator's file.
func decodeStrictly(node *yaml.Node, path string, target any) error {
	if key, line := unknownKey(node, reflect.TypeOf(target).Elem(), path); key != "" {
		if moved, retired := retiredKeys[key]; retired {
			return fmt.Errorf("line %d: %s: this build no longer reads that key. %s", line, key, moved)
		}

		return fmt.Errorf("line %d: %s: this build does not read that key", line, key)
	}

	if node.Tag == nullTag {
		return nil
	}

	return node.Decode(target)
}

// unknownKey returns the first key under node that target has no field for,
// with the line it is written on. An empty key means every key is read.
//
// A node whose shape does not match the type is left alone. The decoder
// reports that against the same nodes, so its line numbers are also the
// operator's file's.
func unknownKey(node *yaml.Node, target reflect.Type, path string) (string, int) {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}

	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}

	if target == yamlNodeType || target.Kind() == reflect.Interface ||
		reflect.PointerTo(target).Implements(yamlUnmarshaler) {
		return "", 0
	}

	switch {
	case node.Kind == yaml.MappingNode && target.Kind() == reflect.Struct:
		return unknownFieldKey(node, target, path)
	case node.Kind == yaml.MappingNode && target.Kind() == reflect.Map:
		return unknownEntryKey(node, target, path)
	case node.Kind == yaml.SequenceNode && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array):
		return unknownElementKey(node, target, path)
	}

	return "", 0
}

// unknownFieldKey holds a mapping to the fields of the struct it fills.
func unknownFieldKey(node *yaml.Node, target reflect.Type, path string) (string, int) {
	fields := decodedFields(target)

	for index := 0; index+1 < len(node.Content); index += nodesPerEntry {
		name := node.Content[index]

		field, read := fields[name.Value]
		if !read {
			return path + "." + name.Value, name.Line
		}

		if key, line := unknownKey(node.Content[index+1], field, path+"."+name.Value); key != "" {
			return key, line
		}
	}

	return "", 0
}

// unknownEntryKey walks a mapping whose keys are names the operator chose, such
// as core.connectors. Only the values have a shape this build can hold them to.
func unknownEntryKey(node *yaml.Node, target reflect.Type, path string) (string, int) {
	for index := 0; index+1 < len(node.Content); index += nodesPerEntry {
		name := node.Content[index]

		if key, line := unknownKey(node.Content[index+1], target.Elem(), path+"."+name.Value); key != "" {
			return key, line
		}
	}

	return "", 0
}

// unknownElementKey walks a sequence and names an element by its position.
func unknownElementKey(node *yaml.Node, target reflect.Type, path string) (string, int) {
	for position, element := range node.Content {
		if key, line := unknownKey(element, target.Elem(), fmt.Sprintf("%s[%d]", path, position)); key != "" {
			return key, line
		}
	}

	return "", 0
}

// decodedFields maps the key a struct reads onto the type behind it, following
// yaml.v3's own rules: the name in the tag, or the lower-cased field name where
// there is no tag, and an inlined struct's fields read as the parent's own.
//
// A field tagged "-" is left out of the map, so naming it is refused like any
// other key this build does not read. This refuses core.persistence: the
// engine is the top-level block, and reading it in both places would let one
// file name two engines.
func decodedFields(target reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, target.NumField())

	for field := range target.Fields() {
		if !field.IsExported() {
			continue
		}

		name, options, _ := strings.Cut(field.Tag.Get("yaml"), ",")

		if strings.Contains(","+options+",", ",inline,") && field.Type.Kind() == reflect.Struct {
			maps.Copy(fields, decodedFields(field.Type))

			continue
		}

		if name == "-" {
			continue
		}

		if name == "" {
			name = strings.ToLower(field.Name)
		}

		fields[name] = field.Type
	}

	return fields
}
