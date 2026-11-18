// Package mqtt publishes domain events to an external MQTT broker on the `v1/`
// topic namespace.
//
// Existing subscribers read that namespace. The topic patterns, the per-topic
// QoS and retain flags, the payload encodings and the publish-only-on-change
// rule are a frozen contract. They are defined in topics.go.
//
// The caller is the event bus, so a call never blocks. The queue is bounded, a
// full queue drops and counts, and Close reports what never arrived.
package mqtt

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/eventsapi"
	"github.com/prusa3d/prusa-connect/sdk/extension"
)

// Type is the `events.type` that selects this adapter.
const Type = "mqtt"

const (
	defaultQueueDepth     = 256
	defaultKeepalive      = 30 * time.Second
	defaultConnectTimeout = 10 * time.Second
	defaultPublishTimeout = 5 * time.Second
	// shutdownGrace is how long Close lets the queue drain. A failed publish
	// is never retried: a retry would reorder events behind a slow broker.
	shutdownGrace = 2 * time.Second
)

//nolint:gochecknoinits // registration has to happen before main; the host reaches it through a blank import.
func init() { eventsapi.Register(Type, doc(), New) }

// doc is this sink's entry in the published extension reference. The patterns
// come from topics.go, so the reference cannot differ from the wire. They are
// declared as destinations.
func doc() extension.Doc {
	return extension.Doc{
		Summary: "Publishes domain events to an MQTT broker on the frozen v1 topic namespace.",
		Description: "The broker is external and the topic patterns, their QoS and their retain flags are " +
			"frozen by the shipping AsyncAPI document. It does not wait for the broker: a sink that refused " +
			"to start because a broker is down would turn an optional integration into a boot dependency, " +
			"so the connection is retried in the background for as long as the process runs.",
		Version:   "1.3.0",
		Contract:  eventsapi.Contract,
		Stability: extension.StabilityStable,
		Publishes: []extension.Destination{
			{
				Address: ServerOnline.Pattern,
				Version: "1.0.0",
				Summary: "Retained. True while this server holds the connection, false through the broker's will.",
			},
			{
				Address: DeviceState.Pattern,
				Version: "1.0.0",
				Summary: "Retained. The printer's state machine value, published when it changes.",
			},
			{
				Address: PrinterOnline.Pattern,
				Version: "1.0.0",
				Summary: "Retained. False when the printer is deleted.",
			},
			{
				Address: CurrentJob.Pattern,
				Version: "1.0.0",
				Summary: "Retained. The printer-reported id of the job in progress, zero when none is.",
			},
			{
				Address: JobState.Pattern,
				Version: "1.0.0",
				Summary: "Retained. A job's state, published once the printer is working on it.",
			},
			{
				Address: ToolActive.Pattern,
				Version: "1.2.0",
				Summary: "Retained. 1 while the tool is active and 0 while it is not, for each head and feeder slot. Empty while not known.",
			},
			{
				Address: ToolMaterial.Pattern,
				Version: "1.2.0",
				Summary: "Retained. The material loaded in the tool, empty while nothing is loaded or it is not known.",
			},
			{
				Address: ToolNozzleTemperature.Pattern,
				Version: "1.2.0",
				Summary: "Not retained. A head's nozzle temperature in whole degrees, empty while not known.",
			},
			{
				Address: ToolNozzleTarget.Pattern,
				Version: "1.2.0",
				Summary: "Not retained. A head's nozzle target in whole degrees, 0 while the heater is off and empty while not known.",
			},
			{
				Address: BedTemperature.Pattern,
				Version: "1.3.0",
				Summary: "Not retained. The bed temperature in whole degrees, empty while not known.",
			},
			{
				Address: BedTarget.Pattern,
				Version: "1.3.0",
				Summary: "Not retained. The bed target in whole degrees, 0 while the heater is off and empty while not known.",
			},
			{
				Address: ChamberTemperature.Pattern,
				Version: "1.3.0",
				Summary: "Not retained. The chamber temperature in whole degrees, on a printer that reports a chamber, empty while not known.",
			},
			{
				Address: FactObserved.Pattern,
				Version: "1.1.0",
				Summary: "Not retained. One durable bus event per message, the envelope as its JSON body.",
			},
		},
		Options: []extension.Option{
			{
				Key:      "url",
				Type:     extension.OptionURL,
				Required: true,
				Summary:  "The broker, as tcp://host:port or ssl://host:port.",
			},
			{
				Key:     "client_id",
				Type:    extension.OptionString,
				Default: "prusa-server-<nanoseconds>",
				Summary: "The MQTT client id. Each sink without one gets a generated id of its own.",
			},
			{Key: "username", Type: extension.OptionString, Summary: "Broker username, when the broker wants one."},
			{Key: "password", Type: extension.OptionSecret, Summary: "Broker password, when the broker wants one."},
			{
				Key:     "tls_skip_verify",
				Type:    extension.OptionBool,
				Default: "false",
				Summary: "Accept any broker certificate. For a lab broker, never for a deployment.",
			},
			{
				Key:     "queue_depth",
				Type:    extension.OptionInt,
				Default: "256",
				Summary: "How many messages may wait for a slow broker before the oldest are dropped.",
			},
			{
				Key:     "keepalive",
				Type:    extension.OptionDuration,
				Default: "30s",
				Summary: "The MQTT keepalive interval.",
			},
			{
				Key:     "connect_timeout",
				Type:    extension.OptionDuration,
				Default: "10s",
				Summary: "How long one connection attempt waits.",
			},
			{
				Key:     "publish_timeout",
				Type:    extension.OptionDuration,
				Default: "5s",
				Summary: "How long one publish waits for its acknowledgement.",
			},
			{
				Key:     "default_action",
				Type:    extension.OptionString,
				Default: "forward",
				Summary: "What happens to a topic no route matches: forward or drop.",
			},
			{
				Key:     "routes",
				Type:    extension.OptionList,
				Summary: "Rewrite rules, each a mapping of from, to and action, applied in order.",
			},
		},
	}
}

// Config is what the operator writes under `events.config`.
type Config struct {
	URL            string
	ClientID       string
	Username       string
	Password       string
	QueueDepth     int
	Keepalive      time.Duration
	ConnectTimeout time.Duration
	PublishTimeout time.Duration
	TLSSkipVerify  bool
	DefaultAction  Action
	Routes         []Route
}

// Processor implements eventsapi.Processor and eventsapi.SampleObserver. It
// maps the typed domain events onto the frozen channels, the tool tree onto
// the tool channels, and every durable bus event onto the fact channel.
type Processor struct {
	*Publisher
}

// New builds a Processor from its spec. It does not wait for the broker, so a
// broker that is down does not stop the server from starting. The connection
// is retried in the background.
func New(spec eventsapi.ProviderSpec) (eventsapi.Processor, error) {
	cfg, err := configFrom(spec.Config)
	if err != nil {
		return nil, err
	}

	broker, err := brokerConfigFrom(cfg)
	if err != nil {
		return nil, err
	}

	p := &Publisher{
		router:  NewRouter(cfg.Routes, cfg.DefaultAction),
		changes: newChangeFilter(),
		queue:   make(chan message, cfg.QueueDepth),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	// Assign the client before start. onConnect reads this field from
	// another goroutine.
	p.client = newClient(broker, p.onConnect, p.onDisconnect)
	p.client.start()

	go p.run()

	return &Processor{Publisher: p}, nil
}

// PrinterStateChanged publishes the printer state machine value.
func (p *Processor) PrinterStateChanged(e eventsapi.PrinterState) {
	p.PublishState(e.ID, e.State)
}

var _ eventsapi.SampleObserver = (*Processor)(nil)

// SamplesObserved publishes the nodes under /tools/ on their tool channels,
// and the bed and chamber temperatures on theirs. A whole tree fills every
// retained channel and clears the channels of a tool that is gone from it.
func (p *Processor) SamplesObserved(samples eventsapi.Samples) {
	for _, node := range samples.Nodes {
		p.PublishTemperatureSample(samples.DeviceID, node.Path, node.Value)
	}

	if samples.Whole {
		p.PublishToolTree(samples.DeviceID, samples.Nodes)

		return
	}

	for _, node := range samples.Nodes {
		p.PublishToolSample(samples.DeviceID, node.Path, node.Value)
	}
}

// PrinterLabelsChanged is not published. The frozen namespace has no label
// channel.
func (p *Processor) PrinterLabelsChanged(eventsapi.PrinterLabels) {}

// PrinterDeleted marks the printer offline and forgets its change-filter
// state. The printer's retained topics stay on the broker.
func (p *Processor) PrinterDeleted(id string) {
	p.PublishOnline(id, false)
	p.changes.forgetPrinter(id)
	p.tools.forget(id)
}

// JobUpdated publishes the job's state and the printer's current job id.
// Nothing is published for a job that has not reached the printer: it has no
// state in the frozen vocabulary.
func (p *Processor) JobUpdated(e eventsapi.JobInfo) {
	state := connectJobState(e.State)
	if state == "" {
		return
	}

	p.PublishJobState(e.PrinterID, e.ExternalID, state)

	if isTerminalJobState(e.State) {
		p.PublishCurrentJob(e.PrinterID, 0)

		return
	}

	p.PublishCurrentJob(e.PrinterID, e.ExternalID)
}

// LabelDeleted is not published, for the reason PrinterLabelsChanged is not.
func (p *Processor) LabelDeleted(string) {}

// FactObserved publishes one durable bus event on the fact channel, with the
// whole envelope as the body. A fact that cannot be marshalled is counted and
// dropped.
func (p *Processor) FactObserved(fact eventsapi.Fact) {
	if fact.Origin.IsZero() || fact.Type == "" {
		return
	}

	body, err := json.Marshal(fact)
	if err != nil {
		p.failed.Add(1)

		return
	}

	p.PublishFact(fact.Origin.Path(), fact.Type, string(body))
}

// connectJobState maps the host's job state onto the vocabulary of the frozen
// per-job channel. An empty result means there is no such state.
func connectJobState(state string) string {
	switch state {
	case "PRINTING":
		return "PRINTING"
	case "PAUSED":
		return "PAUSED"
	case "FINISHED":
		return "FIN_OK"
	case "STOPPED":
		return "FIN_STOPPED"
	case "FAILED", "ABANDONED":
		return "FIN_ERROR"
	default:
		return ""
	}
}

func isTerminalJobState(state string) bool {
	switch state {
	case "FINISHED", "FAILED", "STOPPED", "ABANDONED":
		return true
	default:
		return false
	}
}

// brokerConfigFrom turns the operator's URL into an address and a TLS setting,
// and sets the will. The broker publishes the will, "0" on the server-online
// topic, when this server stops answering keepalives.
func brokerConfigFrom(cfg Config) (brokerConfig, error) {
	address, secure, err := brokerAddress(cfg.URL)
	if err != nil {
		return brokerConfig{}, err
	}

	broker := brokerConfig{
		address:        address,
		clientID:       cfg.ClientID,
		creds:          credentials{username: cfg.Username, password: cfg.Password},
		keepalive:      cfg.Keepalive,
		connectTimeout: cfg.ConnectTimeout,
		publishTimeout: cfg.PublishTimeout,
		will: will{
			topic:   ServerOnline.Pattern,
			payload: boolPayload(false),
			qos:     ServerOnline.QoS,
			retain:  ServerOnline.Retain,
		},
	}

	if secure {
		//nolint:gosec // G402: skipping verification is the operator's choice, for a broker behind a self-signed certificate. It defaults to off.
		broker.tls = &tls.Config{InsecureSkipVerify: cfg.TLSSkipVerify, MinVersion: tls.VersionTLS12}
	}

	return broker, nil
}

// brokerAddress accepts `tcp://` and `ssl://`, the aliases `mqtt://`,
// `tls://` and `mqtts://`, and a bare host:port.
func brokerAddress(raw string) (address string, secure bool, err error) {
	scheme, rest, found := strings.Cut(raw, "://")
	if !found {
		scheme, rest = "tcp", raw
	}

	var defaultPort string

	switch scheme {
	case "tcp", "mqtt":
		defaultPort = "1883"
	case "ssl", "tls", "mqtts":
		secure, defaultPort = true, "8883"
	default:
		return "", false, fmt.Errorf(
			"mqtt: config.url scheme %q is not supported, use tcp:// or ssl://", scheme)
	}

	host, port, splitErr := net.SplitHostPort(rest)
	if splitErr != nil {
		host, port = rest, defaultPort
	}

	if host == "" {
		return "", false, fmt.Errorf("mqtt: config.url %q names no host", raw)
	}

	return net.JoinHostPort(host, port), secure, nil
}
