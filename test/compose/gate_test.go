// Package compose_test is the joint gate ADR-20260901-b2z rule 1 and ADR-20260901-7t4 owe
// together: a module this repository does not compile, linked into a binary by
// a build configuration alone, registering through a published contract and
// served under the namespace its own module path derives.
//
// It is one test because the claims are one claim. Composing without checking
// what came out proves that go build works; checking a registry without having
// composed proves nothing about composition. The gate builds the binary, reads
// the module graph the linker embedded, starts the binary against a
// configuration that declares an instance of the fixture's sink, and reads the
// inventory the running server prints.
//
// It is skipped under -short. A compose is a full build of the core from a
// scratch module, which is a minute, and `make test-unit` runs short.
package compose_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	// fixtureModule is the adapter module the build configuration names. It is
	// in no import list in this tree.
	fixtureModule = "example.com/composefixture"
	// fixtureQualified is the name the host derives for it: the module path,
	// host reversed, "aux" where the host ends. Nothing in the fixture chooses
	// it and nothing in the core writes it down.
	fixtureQualified = "sink/com.example.aux.composefixture/ledger"
	// fixtureKind is the device kind the fixture declares, qualified the same
	// way: the module path derives the namespace and the adapter chose only the
	// bare name.
	fixtureKind = "com.example.aux.composefixture/filament-shelf"
	// inventoryLine is what the server logs once its type table is built.
	inventoryLine = "resource types registered"
	// kindsLine is what it logs once the verb table's per-build half is
	// installed. It is the second half of ADR-20260901-b2z rule 1: an adapter's device
	// class, with verbs of its own, in the running binary's verb table with no
	// edit to core source.
	kindsLine = "device kinds registered"
	// gateAddr is where the composed server binds for this gate, and gateBase is
	// the same address as a caller writes it. They are one constant each so the
	// flag the server is started with and the URL the assertions read cannot
	// drift apart.
	gateAddr = "127.0.0.1:18099"
	gateBase = "http://" + gateAddr
	// startupBudget bounds the wait for that line. The server binds its
	// listeners after it, so a binary that never prints it is a failure and not
	// a slow machine.
	startupBudget = 90 * time.Second
	// buildBudget bounds the compose. It is a full build of the core.
	buildBudget = 10 * time.Minute
)

// TestAComposedAdapterIsLinkedAndServedWithoutTouchingTheCore is the gate.
func TestAComposedAdapterIsLinkedAndServedWithoutTouchingTheCore(t *testing.T) {
	if testing.Short() {
		t.Skip("composing builds the core from a scratch module; run without -short")
	}

	root := repoRoot(t)
	binary := composedBinary(t, "server-composed")

	compose(t, root, binary)

	assertLinked(t, binary)
	assertServed(t, root, binary)
	assertCoreUntouched(t, root)
}

// compose runs the build tool over test/compose/build.yaml.
func compose(t *testing.T, root, binary string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), buildBudget)
	defer cancel()

	build := exec.CommandContext(ctx, "go", "run", "./cmd/compose-build",
		"--config", filepath.Join("test", "compose", "build.yaml"),
		"--output", binary)
	build.Dir = root

	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("composing a binary from test/compose/build.yaml failed.\n%s\n%v", out, err)
	}
}

// assertLinked reads the build graph the linker embedded. A composed module is
// in it by path and by version, which is what ADR-20260901-7t4 rule 10 says a binary
// can be asked about rather than remembered.
func assertLinked(t *testing.T, binary string) {
	t.Helper()

	listed := exec.CommandContext(t.Context(), "go", "version", "-m", binary)

	out, err := listed.Output()
	if err != nil {
		t.Fatalf("reading the composed binary's module graph: %v", err)
	}

	if !strings.Contains(string(out), fixtureModule) {
		t.Errorf("the composed binary's module graph does not name %s.\n%s", fixtureModule, out)
	}
}

// assertServed starts the binary against a configuration declaring an instance
// of the fixture's sink and reads the type table it prints.
//
// The instance matters as much as the name in the table: a registration that
// reached the sdk registry and not the resource plane would still be printed by
// a host that only listed what it linked, and an operator would find out at the
// first start that the type resolves to nothing.
func assertServed(t *testing.T, root, binary string) {
	t.Helper()

	work := t.TempDir()
	config := filepath.Join(work, "config.yaml")

	write(t, config, configWith(t, filepath.Join(root, "config", "config.yaml"), filepath.Join(work, "ledger.jsonl")))

	ctx, cancel := context.WithTimeout(t.Context(), startupBudget)
	defer cancel()

	server := exec.CommandContext(ctx, binary, "--config", config,
		"--addr", gateAddr, "--log-format", "text")
	server.Dir = work
	server.Cancel = func() error { return server.Process.Kill() }

	output, err := server.StdoutPipe()
	if err != nil {
		t.Fatalf("reading the composed server's output: %v", err)
	}

	server.Stderr = server.Stdout

	if err = server.Start(); err != nil {
		t.Fatalf("starting the composed server: %v", err)
	}

	defer func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	}()

	var transcript strings.Builder

	seenTypes, seenKinds := false, false

	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		transcript.WriteString(line + "\n")

		if strings.Contains(line, inventoryLine) {
			if !strings.Contains(line, fixtureQualified) {
				t.Fatalf("the composed server's type table does not carry %s.\n%s", fixtureQualified, line)
			}

			seenTypes = true
		}

		if strings.Contains(line, kindsLine) {
			if !strings.Contains(line, fixtureKind) {
				t.Fatalf("the composed server's verb table does not carry the device kind %s.\n%s",
					fixtureKind, line)
			}

			seenKinds = true
		}

		if seenTypes && seenKinds {
			assertVerbsServed(t)

			return
		}
	}

	t.Fatalf("the composed server never printed both %q and %q.\n%s",
		inventoryLine, kindsLine, transcript.String())
}

// assertVerbsServed reads the running binary's own authorization vocabulary and
// requires the fixture's verbs to be in it, with the class each declared and
// the marking the host computed.
//
// The marking is the point of the second assertion. write_label declares no
// marking and takes a free string with no supported set, so the host marks it
// opaque_payload whatever the declaration says. That is the one check a lying
// declaration cannot route around, and this is it running in a real server.
func assertVerbsServed(t *testing.T) {
	t.Helper()

	settleFirstRun(t, gateBase)

	body := fetch(t, gateBase+"/api/v1/vocabularies/authorization")

	var served struct {
		Kinds []struct {
			Kind   string `json:"kind"`
			Module string `json:"module"`
			Verbs  []struct {
				Verb     string `json:"verb"`
				Class    string `json:"class"`
				Markings []struct {
					Marking string `json:"marking"`
					Marker  string `json:"marker"`
				} `json:"markings"`
			} `json:"verbs"`
		} `json:"kinds"`
	}

	if err := json.Unmarshal(body, &served); err != nil {
		t.Fatalf("reading the served authorization vocabulary: %v\n%s", err, body)
	}

	classes := map[string]string{}
	markings := map[string]string{}

	for _, kind := range served.Kinds {
		if kind.Kind != fixtureKind {
			continue
		}

		if kind.Module != fixtureModule {
			t.Errorf("the served kind names module %q and the fixture is %q", kind.Module, fixtureModule)
		}

		for _, verb := range kind.Verbs {
			classes[verb.Verb] = verb.Class

			for _, marking := range verb.Markings {
				markings[verb.Verb] = marking.Marking + ":" + marking.Marker
			}
		}
	}

	want := map[string]string{
		"com.example.aux.composefixture.command.set_slot_attention": "operate",
		"com.example.aux.composefixture.command.calibrate_scale":    "configure",
		"com.example.aux.composefixture.command.factory_reset":      "configure",
		"com.example.aux.composefixture.command.write_label":        "operate",
	}

	for verb, class := range want {
		if classes[verb] != class {
			t.Errorf("the served vocabulary classes %s as %q and the declaration says %q",
				verb, classes[verb], class)
		}
	}

	if got := markings["com.example.aux.composefixture.command.factory_reset"]; got != "irreversible:adapter" {
		t.Errorf("factory_reset is marked %q, and the adapter marked it irreversible", got)
	}

	if got := markings["com.example.aux.composefixture.command.write_label"]; got != "opaque_payload:host" {
		t.Errorf("write_label is marked %q. Its argument is an unbounded string, so the host marks "+
			"it opaque_payload whatever the declaration says", got)
	}
}

// setupCode is the first-run code both composed servers are configured with,
// the way an automated install sets core.auth.setup_code.
const setupCode = "COMPOSE-GATE-SETUP"

// settleFirstRun records what this server does about identity.
//
// An install that has recorded nothing answers SETUP_PENDING to every operation
// but the probes and the first run itself, so a test that read anything else
// would be decoding a refusal. This is the call an operator makes on the
// first-run page, and every assertion below it starts where a deployment does.
//
// The decision is open: every request is served as the configured principal and
// no credential is asked for, which is what this file and the one beside it
// assume. A decision already recorded is left alone, because recording a second
// one is refused.
func settleFirstRun(t *testing.T, base string) {
	t.Helper()

	var state struct {
		State string `json:"state"`
	}

	if err := json.Unmarshal(fetch(t, base+"/api/v1/setup"), &state); err != nil {
		t.Fatalf("reading what the composed server does about identity: %v", err)
	}

	if state.State != "pending" {
		return
	}

	recorded := request(t, http.MethodPost, base+"/api/v1/setup",
		"application/json", []byte(`{"mode":"open","code":"`+setupCode+`"}`), nil)

	if recorded.status != http.StatusOK {
		t.Fatalf("recording the composed server's first run answered %d: %s",
			recorded.status, recorded.body)
	}
}

// fetch reads one URL from the running server, waiting for it to bind, and
// fails on an answer that is not a success.
//
// The status check is the part worth having. A refusal body decodes into an
// empty document without complaint, so a server that answered 503 to every
// route read as a server whose verb table had lost the composed kind, and the
// gate pointed at the wrong package.
func fetch(t *testing.T, url string) []byte {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)

	for {
		response, err := http.Get(url)
		if err == nil {
			defer func() { _ = response.Body.Close() }()

			body, readErr := io.ReadAll(response.Body)
			if readErr != nil {
				t.Fatalf("reading %s: %v", url, readErr)
			}

			if response.StatusCode != http.StatusOK {
				t.Fatalf("the composed server answered %d to %s: %s", response.StatusCode, url, body)
			}

			return body
		}

		if time.Now().After(deadline) {
			t.Fatalf("the composed server never answered %s: %v", url, err)
		}

		time.Sleep(200 * time.Millisecond)
	}
}

// composedBinary is where one test's composed binary is written. The name
// carries the platform's extension because Windows runs no file without one: a
// binary built as `server-composed` there is found on disk and refused as not
// executable.
func composedBinary(t *testing.T, name string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	return filepath.Join(t.TempDir(), name)
}

// assertCoreUntouched is ADR-20260901-7t4 rule 1's measure, mechanically. Composing an
// adapter is a build configuration and nothing else, so a core checkout it ran
// in is unchanged.
func assertCoreUntouched(t *testing.T, root string) {
	t.Helper()

	status := exec.CommandContext(t.Context(), "git", "status", "--porcelain",
		"--", "internal", "Makefile", ".github", "cmd", "sdk", "adapters")
	status.Dir = root

	out, err := status.Output()
	if err != nil {
		t.Skipf("git status is unavailable here, so rule 1's measure cannot be taken: %v", err)
	}

	if dirty := strings.TrimSpace(string(out)); dirty != "" {
		t.Errorf("composing changed the core checkout, and ADR-20260901-7t4 rule 1 says it changes nothing:\n%s", dirty)
	}
}

// configWith is the shipped configuration with one sink instance added: the
// fixture's, by the bare name the registry resolves.
func configWith(t *testing.T, shipped, ledger string) string {
	t.Helper()

	content, err := os.ReadFile(shipped)
	if err != nil {
		t.Fatalf("reading %s: %v", shipped, err)
	}

	// The shipped configuration declares sinks of its own, so the probe joins
	// that block: a second `sink:` key under `resources:` is a duplicate the
	// loader refuses, and the server exits before it binds.
	const sinks = "\n  sink:\n"

	instance := "    probe:\n      type: ledger\n      enabled: true\n" +
		"      config:\n        path: " + filepath.ToSlash(ledger) + "\n"

	// The setup code joins the shipped auth block for the reason the probe joins
	// the sinks: a second auth key under core is a duplicate. The shipped file
	// leaves core.auth commented out so the wizard can set it, so the block is
	// added under core when there is none.
	const auth = "\n  auth:\n"

	withCode := strings.Replace(string(content), auth, auth+"    setup_code: "+setupCode+"\n", 1)
	if withCode == string(content) {
		const core = "\ncore:\n"

		withCode = strings.Replace(string(content), core, core+"  auth:\n    setup_code: "+setupCode+"\n", 1)
	}

	if withCode == string(content) {
		t.Fatalf("%s carries no core block for this gate to set the setup code in", shipped)
	}

	content = []byte(withCode)

	replaced := strings.Replace(string(content), sinks, sinks+instance, 1)
	if replaced == string(content) {
		const resources = "\nresources:\n"

		replaced = strings.Replace(string(content), resources, resources+"  sink:\n"+instance, 1)
	}

	if replaced == string(content) {
		t.Fatalf("%s carries no resources block for this gate to declare an instance in", shipped)
	}

	// The shipped file announces the server on the local network. The gate
	// reaches the binary over loopback alone, so the announcement is off.
	const announced = "\n  discovery:\n    enabled: true\n"
	if !strings.Contains(replaced, announced) {
		t.Fatalf("%s carries no core.discovery block with enabled: true for this gate to switch off", shipped)
	}

	return strings.Replace(replaced, announced, "\n  discovery:\n    enabled: false\n", 1)
}

// write puts one file where the test wants it.
func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// repoRoot is the checkout this gate composes.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}

		dir = parent
	}
}
