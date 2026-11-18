package app

import (
	"crypto/rand"
	"maps"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mqttclient "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/printer/simulator"
)

// The tool channels end to end (ADR-20261025-frz): a simulated printer reports its
// tool tree, the core holds it, the MQTT sink is handed each node as a sample,
// and a subscriber on a real broker reads a channel per head and per feeder
// slot, under the tree's own ids.
//
// It needs the broker fixture, as the adapter's own broker cases do, and
// skips when nothing answers. `make test-adapters` starts the fixture and runs
// this case.
//
//	docker compose -f docker-compose.local.yml up -d --wait mosquitto-fixture
const (
	mqttBrokerEnv     = "PCL_MQTT_TEST_BROKER"
	mqttDefaultBroker = "127.0.0.1:51883"
)

// mqttFixture finds the broker, or skips.
func mqttFixture(t *testing.T) string {
	t.Helper()

	address := os.Getenv(mqttBrokerEnv)
	if address == "" {
		address = mqttDefaultBroker
	}

	socket, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		t.Skipf("no MQTT broker at %s, set %s to point at one: %v", address, mqttBrokerEnv, err)
	}

	_ = socket.Close()

	return address
}

// toolChannels records the last payload a subscriber read on each topic.
type toolChannels struct {
	mu   sync.Mutex
	last map[string]string
}

func (c *toolChannels) put(topic, payload string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.last[topic] = payload
}

func (c *toolChannels) read() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return maps.Clone(c.last)
}

// subscribeTools subscribes to every topic under root on the broker and
// records the last payload of each, by its topic under root.
func subscribeTools(t *testing.T, address, root string) *toolChannels {
	t.Helper()

	seen := &toolChannels{last: map[string]string{}}

	options := mqttclient.NewClientOptions().
		AddBroker("tcp://" + address).
		SetClientID("app-tools-" + rand.Text()[:8]).
		SetConnectTimeout(5 * time.Second)

	client := mqttclient.NewClient(options)

	connected := client.Connect()
	require.True(t, connected.WaitTimeout(5*time.Second), "the subscriber did not connect")
	require.NoError(t, connected.Error())

	t.Cleanup(func() { client.Disconnect(250) })

	subscribed := client.Subscribe(root+"#", 1, func(_ mqttclient.Client, message mqttclient.Message) {
		seen.put(strings.TrimPrefix(message.Topic(), root), string(message.Payload()))
	})
	require.True(t, subscribed.WaitTimeout(5*time.Second), "the subscription was not acknowledged")
	require.NoError(t, subscribed.Error())

	return seen
}

// retainedTools is what a subscriber that arrives now reads under root: the
// retained messages the broker holds and nothing else. The broker sends them
// at once, and a second is long enough to have read them all.
func retainedTools(t *testing.T, address, root string) map[string]string {
	t.Helper()

	late := subscribeTools(t, address, root)

	time.Sleep(time.Second)

	got := late.read()

	// A reading the sink publishes in that second is not retained. Only the
	// tool channels that are retained are kept.
	maps.DeleteFunc(got, func(topic, _ string) bool { return strings.Contains(topic, "/temp/") })

	return got
}

// requireCleared waits until each topic has carried an empty payload, which is
// how the sink says a value is not known.
func requireCleared(t *testing.T, seen *toolChannels, topics []string, what string) {
	t.Helper()

	require.Eventually(t, func() bool {
		got := seen.read()
		for _, topic := range topics {
			if payload, arrived := got[topic]; !arrived || payload != "" {
				return false
			}
		}

		return true
	}, 3*settle, 50*time.Millisecond, "%s: the channels were never cleared: %v", what, seen.read())
}

func TestASimulatedPrintersToolsReachTheBrokerPerHeadAndPerSlot(t *testing.T) {
	t.Parallel()

	address := mqttFixture(t)

	running := start(t, t.TempDir(), lines(
		"core:", "  log:", "    level: error",
		"resources:", "  sink:", "    "+sinkName+":",
		"      type: mqtt",
		"      enabled: true",
		"      config:",
		"        url: tcp://"+address))

	requireSinkReady(t, running)

	// A serial of its own, because the broker keeps the retained channels of
	// every run before this one.
	sim := dialInSimulator(t, running, simulator.Config{
		Model: "MK4S", MMU: true, Serial: "SIM-MQTT-" + rand.Text()[:8],
	})

	printer := toolsOfListing(t, running, "0", "0.0")

	root := "v1/devices/printers/" + printer.ID + "/data/"
	seen := subscribeTools(t, address, root)

	// The grant comes after the subscription, so the subscriber reads each
	// value as it is published, the empty material of slot 4 among them. The
	// grant is also what makes the core hand the sink the printer's whole
	// tree, which is the half under test for the nodes that do not change.
	grantSink(t, running, printer.ID)

	// Head 0 carries the MMU, so it has all four channels, and each of the
	// five slots has its material and its active channel.
	want := []string{
		"tools/0/active", "tools/0/temp/nozzle/current", "tools/0/temp/nozzle/target",
	}

	var slots []string

	for slot := range 5 {
		id := "tools/0." + strconv.Itoa(slot)
		slots = append(slots, path.Join(id, "active"), path.Join(id, "material"))
	}

	want = append(want, slots...)

	require.Eventually(t, func() bool {
		got := seen.read()
		for _, topic := range want {
			if _, arrived := got[topic]; !arrived {
				return false
			}
		}

		return true
	}, 3*settle, 50*time.Millisecond, "the tool channels never all arrived")

	got := seen.read()

	require.Equal(t, "1", got["tools/0/active"], "the one head is active")

	nozzle, err := strconv.Atoi(got["tools/0/temp/nozzle/current"])
	require.NoError(t, err, "the nozzle reading is a whole number of degrees")
	require.Positive(t, nozzle)

	// The printer is idle, so its heater is off, which is a target of 0.
	require.Equal(t, "0", got["tools/0/temp/nozzle/target"], "an off heater is a target of 0")

	loaded := 0

	// The simulated MMU loads PLA, PETG, PLA and ASA, and leaves slot 4 empty,
	// which is an empty payload.
	materials := []string{"PLA", "PETG", "PLA", "ASA", ""}

	for slot := range 5 {
		id := "tools/0." + strconv.Itoa(slot)
		active, material := path.Join(id, "active"), path.Join(id, "material")
		require.Contains(t, []string{"0", "1"}, got[active], active)

		if got[active] == "1" {
			loaded++
		}

		require.Equal(t, materials[slot], got[material], material)
	}

	require.LessOrEqual(t, loaded, 1, "one slot at most is loaded into the head")

	for topic, payload := range got {
		require.True(t, strings.HasPrefix(topic, "tools/") || strings.HasPrefix(topic, "temp/") ||
			!strings.Contains(topic, "/"),
			"%s is not a tool channel, a temperature channel or a printer channel", topic)
		require.NotContains(t, topic, "slot/", "a retired slot channel was published")
		require.NotEqual(t, "material", topic, "the retired printer-wide material channel was published")
		require.NotEqual(t, "null", payload, "%s carries the string null", topic)
	}

	// The MMU is switched off. Its five slots leave the tree, and each of
	// their channels is cleared, retained ones included: a subscriber that
	// arrives later reads the head and no slot.
	require.True(t, sim.SetMMU(false), "the printer did not send its INFO")

	requireCleared(t, seen, slots, "the MMU switched off")

	retained := retainedTools(t, address, root)
	require.Equal(t, "1", retained["tools/0/active"], "the head stays on the broker")

	for _, topic := range slots {
		require.NotContains(t, retained, topic, "the broker still holds a slot of the MMU that is off")
	}
}

// TestASimulatedPrintersToolsReachTheBrokerAndADisabledHeadLeavesIt is the
// head a person switches off on an XL: each of its channels is cleared, and
// the broker no longer holds its retained ones.
func TestASimulatedPrintersToolsReachTheBrokerAndADisabledHeadLeavesIt(t *testing.T) {
	t.Parallel()

	address := mqttFixture(t)

	running := start(t, t.TempDir(), lines(
		"core:", "  log:", "    level: error",
		"resources:", "  sink:", "    "+sinkName+":",
		"      type: mqtt",
		"      enabled: true",
		"      config:",
		"        url: tcp://"+address))

	requireSinkReady(t, running)

	sim := dialInSimulator(t, running, simulator.Config{Model: "XL", Serial: "SIM-MQTT-" + rand.Text()[:8]})

	printer := toolsOfListing(t, running, "0", "1")

	root := "v1/devices/printers/" + printer.ID + "/data/"
	seen := subscribeTools(t, address, root)

	grantSink(t, running, printer.ID)

	head := []string{"tools/1/active", "tools/1/material"}

	require.Eventually(t, func() bool {
		got := seen.read()

		return got["tools/1/active"] == "0" && got["tools/1/material"] != ""
	}, 3*settle, 50*time.Millisecond, "head 1 never reached the broker: %v", seen.read())

	require.True(t, sim.SetDisabledHeads(1), "the printer did not send its INFO")

	requireCleared(t, seen, append(head, "tools/1/temp/nozzle/current"), "head 1 switched off")

	retained := retainedTools(t, address, root)
	require.Equal(t, "1", retained["tools/0/active"], "head 0 stays on the broker")

	for _, topic := range head {
		require.NotContains(t, retained, topic, "the broker still holds a channel of the head that is off")
	}
}
