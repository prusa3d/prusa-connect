package mqtt

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/sdk/eventsapi"
)

// These run against a real broker. A sink verified against a fake broker
// proves the fake, and the frozen properties here are all properties of what a
// broker does with a publish: whether it stores it, at which QoS it hands it
// on, and whether it fires a will.
//
//	docker compose -f docker-compose.local.yml up -d mosquitto-fixture
//	  -v "$PWD/mosquitto.conf:/mosquitto/config/mosquitto.conf:ro" eclipse-mosquitto:2
//
// with mosquitto.conf holding `listener 1883`, `allow_anonymous true` and
// `persistence false`. Point PCL_MQTT_TEST_BROKER at it, or leave it unset and
// a broker on 127.0.0.1:51883 is found on its own.
const brokerEnv = "PCL_MQTT_TEST_BROKER"

const defaultTestBroker = "127.0.0.1:51883"

// fixtureAddress is where the broker is, overridable for a run that published a
// different port.
func fixtureAddress() string {
	if address := os.Getenv(brokerEnv); address != "" {
		return address
	}

	return defaultTestBroker
}

// testBroker finds the fixture to test against. It skips when nothing answers,
// so a developer without Docker still gets the rest of the suite, and it fails
// when something answers that is not the fixture: a broker that is not this one
// accepts every publish the suite makes and the run goes green against
// somebody else's data. See fixture_test.go.
func testBroker(t *testing.T) string {
	t.Helper()

	address := fixtureAddress()

	socket, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		t.Skipf("no MQTT broker at %s, set %s to point at one: %v", address, brokerEnv, err)
	}

	require.NoError(t, socket.Close())

	declared, err := declaredVersion()
	require.NoError(t, err)

	announced, err := brokerVersion()
	if err != nil {
		t.Fatal(foreignBroker(address, "answers nothing an MQTT client can read: "+err.Error(), declared))
	}

	require.NoError(t, checkBrokerIsOurs(address, announced, declared))

	return address
}

// uniquePrinter keeps one test's retained topics out of another's, because a
// broker remembers them and the suite runs in a random order.
func uniquePrinter(t *testing.T) string {
	t.Helper()

	return uniqueName("printer-")
}

// uniqueName returns the prefix followed by random characters. Test processes
// share one broker, and some clocks move in steps of half a millisecond, so a
// name made from the time can repeat in another process.
func uniqueName(prefix string) string {
	return prefix + strings.ToLower(rand.Text())
}

func connectedProcessor(t *testing.T, address string) *Processor {
	t.Helper()

	// The broker drops the older connection when a second client connects
	// with the same id, so each processor here gets an id no other test
	// process can generate.
	config := map[string]any{"url": "tcp://" + address, "client_id": uniqueName("prusa-server-test-")}

	built, err := New(eventsapi.ProviderSpec{Type: Type, Config: config})
	require.NoError(t, err)

	p, ok := built.(*Processor)
	require.True(t, ok)

	t.Cleanup(func() { _ = p.Close() })

	deadline := time.Now().Add(10 * time.Second)
	for !p.client.connected() {
		if time.Now().After(deadline) {
			t.Fatal("the adapter never connected to the broker")
		}

		time.Sleep(20 * time.Millisecond)
	}

	return p
}

// TestAnEventReachesTheFrozenTopicWithItsFlags is the first proof: a domain
// event goes in, and a subscriber sees it on the topic section 7 names, at the
// QoS and with the retain flag section 7 assigns.
func TestAnEventReachesTheFrozenTopicWithItsFlags(t *testing.T) {
	address := testBroker(t)
	printer := uniquePrinter(t)

	p := connectedProcessor(t, address)
	p.PrinterStateChanged(eventsapi.PrinterState{ID: printer, State: "PRINTING"})

	// Subscribing after the publish is what proves retention: a broker hands a
	// stored message to a new subscriber with the retain flag set, and hands a
	// live one on with the flag cleared however it was published.
	observer := subscribeAt(t, address, DeviceState.For(printer))

	messages := observer.await(t, 1)
	require.Len(t, messages, 1)

	assert.Equal(t, "v1/devices/printers/"+printer+"/data/state", messages[0].topic)
	assert.Equal(t, "PRINTING", messages[0].payload)
	assert.Equal(t, byte(1), messages[0].qos)
	assert.True(t, messages[0].retain, "the state topic is retained, so a dashboard opened later shows it")

	t.Logf("observed %+v", messages[0])
}

// TestEveryFrozenChannelLandsWhereTheContractSaysItDoes walks the whole table
// through a broker. The QoS a subscriber is handed is capped by its own
// subscription, so the observer subscribes at 2 and what arrives is what the
// adapter published.
func TestEveryFrozenChannelLandsWhereTheContractSaysItDoes(t *testing.T) {
	address := testBroker(t)
	printer := uniquePrinter(t)

	p := connectedProcessor(t, address)

	live := subscribeAt(t, address, Base+"/"+printer+"/#")

	publishOneOfEach(p.Publisher, printer)

	messages := live.await(t, len(Topics)-1)

	byTopic := make(map[string]observed, len(messages))
	for _, message := range messages {
		byTopic[message.topic] = message
	}

	for _, topic := range Topics {
		if topic.Name == ServerOnline.Name {
			continue
		}

		resolved := resolveForTest(topic, printer)

		message, arrived := byTopic[resolved]
		if !assert.True(t, arrived, "nothing arrived on %s (%s)", resolved, topic.Name) {
			continue
		}

		assert.Equal(t, topic.QoS, message.qos, "QoS on %s", resolved)
	}

	// A second subscriber, arriving after the fact, sees exactly the topics
	// the contract retains and none of the ones it does not.
	late := subscribeAt(t, address, Base+"/"+printer+"/#")
	late.settle()

	retained := map[string]bool{}
	for _, message := range late.seen() {
		retained[message.topic] = true
		assert.True(t, message.retain, "%s was stored, so it should carry the retain flag", message.topic)
	}

	for _, topic := range Topics {
		if topic.Name == ServerOnline.Name {
			continue
		}

		resolved := resolveForTest(topic, printer)
		assert.Equal(t, topic.Retain, retained[resolved], "retention of %s (%s)", resolved, topic.Name)
	}

	t.Logf("%d channels observed live, %d of them retained", len(byTopic), len(retained))
}

// publishOneOfEach sends one message on every per-printer channel, which is
// what makes the assertion above a statement about the whole table rather than
// about the handful of channels sdk/eventsapi can currently feed.
func publishOneOfEach(p *Publisher, printer string) {
	p.PublishState(printer, "PRINTING")
	p.PublishOnline(printer, true)
	p.PublishSpeed(printer, 100)
	p.PublishTemperatureSample(printer, "/temperature/bed", json.RawMessage(`59.6`))
	p.PublishTemperatureSample(printer, "/temperature/bed_target", json.RawMessage(`60`))
	p.PublishTemperatureSample(printer, "/temperature/chamber", json.RawMessage(`31.2`))
	p.PublishAxis(printer, AxisX, 10.5)
	p.PublishAxis(printer, AxisY, 20.25)
	p.PublishAxis(printer, AxisZ, 0.2)
	p.PublishBlockingReasonsChanged(printer)
	p.PublishCurrentJob(printer, 42)
	p.PublishJobState(printer, 42, "PRINTING")
	p.PublishJobProgress(printer, 42, 37)
	p.PublishJobTimeRemaining(printer, 42, 1800)
	p.PublishDialog(printer, `{"id":1,"code":"12345","text":"t","title":"T","buttons":["OK"],"key":"k"}`)
	p.PublishToolSample(printer, "/tools/1/active", json.RawMessage(`true`))
	p.PublishToolSample(printer, "/tools/1/material_package_instance/package/material/abbreviation",
		json.RawMessage(`"PETG"`))
	p.PublishToolSample(printer, "/tools/1/nozzle_temp", json.RawMessage(`205.7`))
	p.PublishToolSample(printer, "/tools/1/nozzle_target_temp", json.RawMessage(`210`))
}

// resolveForTest fills every placeholder a channel might carry with the values
// publishOneOfEach used.
func resolveForTest(topic Topic, printer string) string {
	resolved := strings.ReplaceAll(topic.Pattern, printerPlaceholder, printer)
	resolved = strings.ReplaceAll(resolved, jobPlaceholder, "42")

	return strings.ReplaceAll(resolved, toolPlaceholder, "1")
}

// TestARepeatedValueIsNotPublishedAgain is publish-only-on-change, observed
// from the far side of a broker rather than from the adapter's counters.
func TestARepeatedValueIsNotPublishedAgain(t *testing.T) {
	address := testBroker(t)
	printer := uniquePrinter(t)

	p := connectedProcessor(t, address)
	observer := subscribeAt(t, address, Base+"/"+printer+"/#")

	p.PrinterStateChanged(eventsapi.PrinterState{ID: printer, State: "PRINTING"})
	p.PrinterStateChanged(eventsapi.PrinterState{ID: printer, State: "PRINTING"})
	p.PrinterStateChanged(eventsapi.PrinterState{ID: printer, State: "PRINTING"})

	for _, reading := range []string{`21.0`, `21.4`, `20.6`, `21.5`} {
		p.SamplesObserved(eventsapi.Samples{
			DeviceID: printer,
			Nodes:    []eventsapi.Sample{{Path: "/tools/0/nozzle_temp", Value: json.RawMessage(reading)}},
		})
	}

	observer.settle()

	states := observer.on(DeviceState.For(printer))
	assert.Len(t, states, 1, "three identical states, one publish")

	temperatures := observer.on(ToolNozzleTemperature.ForTool(printer, "0"))
	require.Len(t, temperatures, 2, "four readings spanning one whole degree, two publishes")
	assert.Equal(t, "21", temperatures[0].payload)
	assert.Equal(t, "22", temperatures[1].payload)

	t.Logf("states: %v", states)
	t.Logf("temperatures: %v", temperatures)
}

// TestTheWillFiresWhenTheConnectionDropsUncleanly severs the socket without a
// DISCONNECT, which is what a crashed or partitioned server looks like to a
// broker. A subscriber reads v1/server/online to know the printer values have
// stopped being current.
func TestTheWillFiresWhenTheConnectionDropsUncleanly(t *testing.T) {
	address := testBroker(t)

	proxy := newSeverableProxy(t, address)

	observer := subscribeAt(t, address, ServerOnline.Pattern)

	built, err := New(eventsapi.ProviderSpec{Type: Type, Config: map[string]any{"url": "tcp://" + proxy.address()}})
	require.NoError(t, err)

	p, ok := built.(*Processor)
	require.True(t, ok)

	t.Cleanup(func() { _ = p.Close() })

	deadline := time.Now().Add(10 * time.Second)
	for !p.client.connected() {
		require.False(t, time.Now().After(deadline), "the adapter never connected through the proxy")
		time.Sleep(20 * time.Millisecond)
	}

	announced := observer.awaitPayloadAfter(t, ServerOnline.Pattern, "1", -1)

	proxy.sever()

	// Nothing was published by the adapter here. The "0" is the broker acting
	// on the will it was given at CONNECT, which is what a subscriber reads to
	// know the printer values have stopped being current.
	fired := observer.awaitPayloadAfter(t, ServerOnline.Pattern, "0", announced)

	// The supervisor reconnects through the proxy's listener and says so,
	// which is the other half of surviving a partition.
	observer.awaitPayloadAfter(t, ServerOnline.Pattern, "1", fired)

	t.Logf("observed on %s: %v", ServerOnline.Pattern, observer.seen())
}

// TestACleanShutdownSaysSoRatherThanLookingLikeACrash covers the other half of
// the lifecycle topic.
func TestACleanShutdownSaysSoRatherThanLookingLikeACrash(t *testing.T) {
	address := testBroker(t)

	observer := subscribeAt(t, address, ServerOnline.Pattern)

	p := connectedProcessor(t, address)

	announced := observer.awaitPayloadAfter(t, ServerOnline.Pattern, "1", -1)

	require.NoError(t, p.Close())
	observer.awaitPayloadAfter(t, ServerOnline.Pattern, "0", announced)

	// The lifecycle topic is retained, so a dashboard that starts after the
	// server stopped is told so rather than left with nothing.
	late := subscribeAt(t, address, ServerOnline.Pattern)

	stored := late.await(t, 1)
	assert.Equal(t, "0", stored[0].payload)
	assert.True(t, stored[0].retain)
	assert.Equal(t, byte(1), stored[0].qos)

	t.Logf("observed on %s: %v, then stored: %+v", ServerOnline.Pattern, observer.seen(), stored[0])
}

// severableProxy forwards TCP to the broker and can cut both halves at once,
// with no DISCONNECT, which is the only way to make a will fire on demand.
type severableProxy struct {
	listener net.Listener

	mu    sync.Mutex
	pairs []net.Conn
}

func newSeverableProxy(t *testing.T, target string) *severableProxy {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxy := &severableProxy{listener: listener}
	t.Cleanup(func() { _ = listener.Close() })

	go proxy.accept(target)

	return proxy
}

func (p *severableProxy) address() string {
	return p.listener.Addr().String()
}

func (p *severableProxy) accept(target string) {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}

		upstream, dialErr := net.DialTimeout("tcp", target, 5*time.Second)
		if dialErr != nil {
			_ = client.Close()

			return
		}

		p.mu.Lock()
		p.pairs = append(p.pairs, client, upstream)
		p.mu.Unlock()

		go func() { _, _ = io.Copy(upstream, client) }()
		go func() { _, _ = io.Copy(client, upstream) }()
	}
}

func (p *severableProxy) sever() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, socket := range p.pairs {
		_ = socket.Close()
	}

	p.pairs = nil
}

// TestTheConnectionSurvivesAnIdleKeepalivePeriod exercises the ping loop.
// A broker disconnects a client that has said nothing for one and a half
// keepalive intervals, so a silent connection held open past that is the only
// evidence that PINGREQ is going out and PINGRESP is coming back.
func TestTheConnectionSurvivesAnIdleKeepalivePeriod(t *testing.T) {
	address := testBroker(t)
	printer := uniquePrinter(t)

	const keepalive = 2 * time.Second

	built, err := New(eventsapi.ProviderSpec{
		Type:   Type,
		Config: map[string]any{"url": "tcp://" + address, "keepalive": "2s"},
	})
	require.NoError(t, err)

	p, ok := built.(*Processor)
	require.True(t, ok)

	t.Cleanup(func() { _ = p.Close() })

	deadline := time.Now().Add(10 * time.Second)
	for !p.client.connected() {
		require.False(t, time.Now().After(deadline), "the adapter never connected")
		time.Sleep(20 * time.Millisecond)
	}

	observer := subscribeAt(t, address, DeviceState.For(printer))

	// Long enough that a client which never pinged would already be gone.
	time.Sleep(3 * keepalive)

	require.True(t, p.client.connected(), "the connection was dropped during an idle keepalive period")

	p.PrinterStateChanged(eventsapi.PrinterState{ID: printer, State: "PRINTING"})

	messages := observer.await(t, 1)
	assert.Equal(t, "PRINTING", messages[0].payload)

	t.Logf("published after %v of silence at a %v keepalive: %+v", 3*keepalive, keepalive, messages[0])
}

// authBrokerEnv points at a broker that refuses anonymous connections, which is
// the only way to tell a username that worked from one that was ignored:
//
//	docker run -d --name pcl-mqtt-auth -p 18831:1883 \
//	  -v "$PWD/mosquitto.conf:/mosquitto/config/mosquitto.conf:ro" \
//	  -v "$PWD/passwd:/mosquitto/config/passwd" eclipse-mosquitto:2 \
//	  sh -c "mosquitto_passwd -U /mosquitto/config/passwd && exec mosquitto -c /mosquitto/config/mosquitto.conf"
//
// with `allow_anonymous false`, a `password_file`, and a passwd file holding
// `prusa:hunter2`.
const (
	authBrokerEnv  = "PCL_MQTT_TEST_BROKER_AUTH"
	authBrokerUser = "PCL_MQTT_TEST_BROKER_USER"
	authBrokerPass = "PCL_MQTT_TEST_BROKER_PASSWORD"
)

func TestCredentialsReachABrokerThatDemandsThem(t *testing.T) {
	address := os.Getenv(authBrokerEnv)
	if address == "" {
		t.Skipf("set %s to a broker with allow_anonymous false to exercise the credential path", authBrokerEnv)
	}

	printer := uniquePrinter(t)
	user, password := os.Getenv(authBrokerUser), os.Getenv(authBrokerPass)

	// Without credentials the broker refuses, so nothing ever connects.
	anonymous, err := New(eventsapi.ProviderSpec{
		Type:   Type,
		Config: map[string]any{"url": "tcp://" + address, "connect_timeout": "2s"},
	})
	require.NoError(t, err)

	time.Sleep(time.Second)

	refused, ok := anonymous.(*Processor)
	require.True(t, ok)
	assert.False(t, refused.client.connected(), "the broker should have refused an anonymous connection")
	require.NoError(t, anonymous.Close())

	// With them it connects and publishes.
	built, err := New(eventsapi.ProviderSpec{
		Type: Type,
		Config: map[string]any{
			"url": "tcp://" + address, "username": user, "password": password,
		},
	})
	require.NoError(t, err)

	p, ok := built.(*Processor)
	require.True(t, ok)

	t.Cleanup(func() { _ = p.Close() })

	deadline := time.Now().Add(10 * time.Second)
	for !p.client.connected() {
		require.False(t, time.Now().After(deadline), "the adapter never connected with credentials")
		time.Sleep(20 * time.Millisecond)
	}

	observer := subscribeWithCredentials(t, address, DeviceState.For(printer), user, password)

	p.PrinterStateChanged(eventsapi.PrinterState{ID: printer, State: "PRINTING"})

	messages := observer.await(t, 1)
	assert.Equal(t, "PRINTING", messages[0].payload)

	t.Logf("published to an authenticated broker: %+v", messages[0])
}
