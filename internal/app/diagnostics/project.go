package diagnostics

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/prusa3d/prusa-connect/internal/app/config"
	"github.com/prusa3d/prusa-connect/internal/transport/http/httpapi"
)

type rule int

const (
	keep rule = iota + 1
	portOnly
	countOnly
	connectorsOnly
	presence
)

// configRules name the keys, by their documented path, whose value is reported
// in some other form than the default: a number, a bool or a duration is kept
// because those are tunables, and any other string is reduced to set or unset.
var configRules = map[string]rule{
	"api.addr":                  portOnly,
	"observability.addr":        portOnly,
	"api.allowed_origins":       countOnly,
	"trusted_proxies":           countOnly,
	"modules_local":             countOnly,
	"connectors":                connectorsOnly,
	"log.level":                 keep,
	"log.format":                keep,
	"frontend.theme":            keep,
	"frontend.default_language": keep,
	"frontend.printer_address":  presence,
}

// sizePattern accepts the byte sizes the configuration writes, such as 64MiB.
var sizePattern = regexp.MustCompile(`^\d+(\.\d+)?\s?([KMGT]i?B|B)$`)

// configBranches are the paths, under core, that the configuration structs
// spell out. A map anywhere else is keyed by whatever the operator wrote, so
// its keys are never reported.
var configBranches = branchesOf(config.Keys)

func branchesOf(keys map[string]config.Key) map[string]bool {
	branches := map[string]bool{}

	for key := range keys {
		path, ok := strings.CutPrefix(key, "core.")
		if !ok {
			continue
		}

		for i := strings.LastIndex(path, "."); i > 0; i = strings.LastIndex(path, ".") {
			path = path[:i]
			branches[path] = true
		}
	}

	return branches
}

func configView(core config.Core, s *Scrubber) map[string]any {
	encoded, err := yaml.Marshal(core)
	if err != nil {
		return map[string]any{"error": "the configuration did not encode"}
	}

	var tree map[string]any
	if err = yaml.Unmarshal(encoded, &tree); err != nil {
		return map[string]any{"error": "the configuration did not decode"}
	}

	view, _ := walk("", tree, s).(map[string]any)
	if view == nil {
		view = map[string]any{}
	}

	// persistence is read from its own top-level block, so Core does not
	// encode it.
	view["persistence"] = map[string]any{
		"type":         core.Persistence.Type,
		"postgres_dsn": presenceOf(core.Persistence.Postgres.DSN),
	}

	return view
}

func walk(path string, node any, s *Scrubber) any {
	switch configRules[path] {
	case keep:
		return node
	case portOnly:
		return portOf(node)
	case countOnly:
		return countOf(node)
	case presence:
		return presenceOf(node)
	case connectorsOnly:
		return connectorsOf(node, s)
	}

	switch v := node.(type) {
	case map[string]any:
		if path != "" && !configBranches[path] {
			return countOf(v)
		}

		out := make(map[string]any, len(v))
		for key, child := range v {
			out[key] = walk(strings.TrimPrefix(path+"."+key, "."), child, s)
		}

		return out
	case []any:
		return countOf(v)
	case int, int64, uint64, float64, bool, nil:
		return v
	case string:
		if _, err := time.ParseDuration(v); err == nil || sizePattern.MatchString(v) {
			return v
		}

		return presenceOf(v)
	default:
		return presenceOf(v)
	}
}

func presenceOf(v any) string {
	if v == nil || v == "" {
		return "unset"
	}

	return "set"
}

func countOf(v any) int {
	switch c := v.(type) {
	case []any:
		return len(c)
	case map[string]any:
		return len(c)
	}

	return 0
}

func portOf(v any) string {
	addr, _ := v.(string)
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return ":" + port
	}

	return presenceOf(addr)
}

// connectorsOf keeps each connector's type and switch, under a hash of the
// name the operator chose. Its config block is the dialect's own and may hold
// a credential, so none of it is reported.
func connectorsOf(v any, s *Scrubber) any {
	connectors, ok := v.(map[string]any)
	if !ok {
		return countOf(v)
	}

	out := map[string]any{}
	for name, raw := range connectors {
		block, _ := raw.(map[string]any)
		out[s.Hash(name)] = map[string]any{"type": block["type"], "enabled": block["enabled"]}
	}

	return out
}

type deviceView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Vendor    string `json:"vendor"`
	Model     string `json:"model"`
	Firmware  string `json:"firmware"`
	Driver    string `json:"driver"`
	Serial    string `json:"serial"`
	UnitUID   string `json:"unit_uid"`
	Connected bool   `json:"connected"`
	Departed  bool   `json:"departed"`
	State     string `json:"state"`
	Busy      bool   `json:"busy"`
}

func devicesView(devices []httpapi.Device, s *Scrubber) []deviceView {
	out := make([]deviceView, 0, len(devices))

	for _, d := range devices {
		var state string
		_ = json.Unmarshal(d.State["/state"], &state) // an absent state stays empty

		out = append(out, deviceView{
			ID: d.ID, Name: s.Hash(d.Name), Vendor: d.Vendor, Model: d.Model, Firmware: d.Firmware,
			Driver: s.Hash(d.Driver), Serial: s.Hash(d.Serial), UnitUID: s.Hash(d.UnitUID),
			Connected: d.Connected, Departed: d.Departed != nil, State: state, Busy: d.Job != nil,
		})
	}

	return out
}

type healthView struct {
	Readiness  readinessView         `json:"readiness"`
	Connectors []connectorHealthView `json:"connectors"`
}

type readinessView struct {
	App       string               `json:"app"`
	Discovery string               `json:"discovery,omitempty"`
	Ready     bool                 `json:"ready"`
	Serving   int                  `json:"serving"`
	Declared  int                  `json:"declared"`
	Blocked   []string             `json:"blocked"`
	Instances []instanceHealthView `json:"instances"`
}

type instanceHealthView struct {
	Resource string `json:"resource"`
	State    string `json:"state"`
	Required bool   `json:"required"`
	Serving  bool   `json:"serving"`
}

type connectorHealthView struct {
	Connector         string             `json:"connector"`
	Type              string             `json:"type,omitempty"`
	Status            string             `json:"status"`
	Counts            map[string]int     `json:"counts"`
	Total             int                `json:"total"`
	Degraded          []deviceHealthView `json:"degraded"`
	DegradedTruncated int                `json:"degraded_truncated"`
}

type deviceHealthView struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	Attempts int    `json:"attempts"`
}

// healthViewOf reports the readiness verdict and the connector rollups. A
// connector instance is named by the operator, so it is hashed as config.json
// hashes it, and its dialect is reported beside it.
func healthViewOf(
	r httpapi.ReadinessReport,
	rollups []httpapi.ConnectorRollup,
	core config.Core,
	s *Scrubber,
) healthView {
	view := healthView{
		Readiness: readinessView{
			App: r.App, Discovery: r.Discovery, Ready: r.Ready, Serving: r.Serving, Declared: r.Declared,
			Blocked: r.Blocked, Instances: make([]instanceHealthView, 0, len(r.Instances)),
		},
		Connectors: make([]connectorHealthView, 0, len(rollups)),
	}

	for _, i := range r.Instances {
		view.Readiness.Instances = append(view.Readiness.Instances, instanceHealthView{
			Resource: i.Resource, State: i.State, Required: i.Required, Serving: i.Serving,
		})
	}

	for _, c := range rollups {
		connector := connectorHealthView{
			Connector: s.Hash(c.Connector), Type: connectorType(core, c.Connector), Status: c.Status,
			Counts: c.Counts, Total: c.Total, DegradedTruncated: c.DegradedTruncated,
			Degraded: make([]deviceHealthView, 0, len(c.Degraded)),
		}

		for _, d := range c.Degraded {
			connector.Degraded = append(connector.Degraded, deviceHealthView{
				ID: d.ID, State: d.State, Reason: d.Reason, Attempts: d.Attempts,
			})
		}

		view.Connectors = append(view.Connectors, connector)
	}

	return view
}

// connectorType is the dialect an instance runs. With no connector declared,
// the shipping dialect is mounted under its own name.
func connectorType(core config.Core, instance string) string {
	if len(core.Connectors) == 0 {
		return instance
	}

	return core.Connectors[instance].Type
}

type conditionView struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type resourceView struct {
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Enabled    bool            `json:"enabled"`
	Required   bool            `json:"required"`
	Module     string          `json:"module,omitempty"`
	Version    string          `json:"module_version,omitempty"`
	State      string          `json:"state"`
	Serving    bool            `json:"serving"`
	Result     string          `json:"result"`
	Attempt    int             `json:"attempt"`
	Config     json.RawMessage `json:"config,omitempty"`
	Conditions []conditionView `json:"conditions,omitempty"`
}

// resourcesView takes instances as the resource store lists them, and reports
// their config by shape alone.
func resourcesView(items []httpapi.ResourceInstance, s *Scrubber) []resourceView {
	out := make([]resourceView, 0, len(items))

	for _, r := range items {
		view := resourceView{
			Kind:     r.Kind,
			Name:     r.Name,
			Type:     r.Type,
			Enabled:  r.Enabled,
			Required: r.Required,
			State:    r.Status.State,
			Serving:  r.Status.Serving,
			Result:   s.Text(r.Status.Result),
			Attempt:  r.Status.Attempt,
			Config:   shapeJSON(json.RawMessage(r.Config)),
		}

		if r.Module != nil {
			view.Module, view.Version = r.Module.Title, r.Module.Version
		}

		for _, c := range r.Status.Conditions {
			view.Conditions = append(view.Conditions, conditionView{
				Type: c.Type, Status: c.Status, Reason: c.Reason, Message: s.Text(c.Message),
			})
		}

		out = append(out, view)
	}

	return out
}

// shapeJSON reports a module's settings as their keys and, for each value,
// its type or that it is unset. An option the module did not declare as a
// secret, such as an api_key written as a plain string or a broker address,
// is as free as any other text, so no value is reported at all.
func shapeJSON(raw json.RawMessage) json.RawMessage {
	var tree any
	if len(raw) == 0 || json.Unmarshal(raw, &tree) != nil {
		return nil
	}

	encoded, err := json.Marshal(shapeOf(tree))
	if err != nil {
		return nil
	}

	return encoded
}

func shapeOf(node any) any {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			v[key] = shapeOf(child)
		}

		return v
	case []any:
		return fmt.Sprintf("list of %d", len(v))
	case string:
		if v == "" {
			return "unset"
		}

		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	default:
		return "unset"
	}
}

// ConfigSecrets are the secret values the core configuration holds, for the
// sweep.
func ConfigSecrets(core config.Core) []string {
	return append([]string{core.Auth.SetupCode}, core.Persistence.Postgres.Passwords()...)
}
