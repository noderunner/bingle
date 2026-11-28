package app_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // coturn's credential scheme
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
)

// coturnImage is the relay the TURN test runs: pinned, so that a new release of
// coturn changes the test only when somebody moves this line.
const coturnImage = "docker.io/coturn/coturn:4.18.0"

// relayOnlyScript runs in every page before the page's own scripts (stepAddScript):
// each RTCPeerConnection the page makes is given iceTransportPolicy "relay", so
// that no direct path can be what connects, and is kept in window.__pcs for the
// test to read its statistics. Only the browser driver adds it; the page has no
// way to ask for it.
const relayOnlyScript = `(() => {
	const Native = window.RTCPeerConnection;
	if (!Native) return;
	window.__pcs = [];
	class RelayOnly extends Native {
		constructor(cfg, ...rest) {
			super(Object.assign({}, cfg || {}, { iceTransportPolicy: "relay" }), ...rest);
			window.__pcs.push(this);
		}
	}
	window.RTCPeerConnection = RelayOnly;
})()`

// relayStatsJS reports, for every connected peer connection of the page, its ICE
// policy, the types of the selected pair's two candidates and the audio received.
const relayStatsJS = `(async () => {
	const out = [];
	for (const pc of window.__pcs || []) {
		if (pc.connectionState !== "connected") continue;
		const stats = await pc.getStats();
		const byId = new Map();
		stats.forEach((r) => byId.set(r.id, r));
		let pair = null;
		stats.forEach((r) => { if (r.type === "transport" && r.selectedCandidatePairId) pair = byId.get(r.selectedCandidatePairId); });
		const local = pair ? byId.get(pair.localCandidateId) : null;
		const remote = pair ? byId.get(pair.remoteCandidateId) : null;
		let bytes = 0, packets = 0;
		stats.forEach((r) => { if (r.type === "inbound-rtp" && r.kind === "audio") { bytes += r.bytesReceived || 0; packets += r.packetsReceived || 0; } });
		out.push({
			policy: pc.getConfiguration().iceTransportPolicy,
			local: local ? local.candidateType : "", remote: remote ? remote.candidateType : "",
			localAddr: local ? local.address + ":" + local.port : "", remoteAddr: remote ? remote.address + ":" + remote.port : "",
			bytes, packets,
		});
	}
	return JSON.stringify(out);
})()`

// gatherJS makes a relay-only peer connection with the iceServers given as its
// argument, gathers its candidates for at most ten seconds and reports the relay
// candidates found and the candidate errors seen.
const gatherJS = `(async (servers) => {
	const pc = new RTCPeerConnection({ iceServers: servers, iceTransportPolicy: "relay" });
	const relays = [], errors = [];
	pc.onicecandidate = (e) => { if (e.candidate && e.candidate.type === "relay") relays.push(e.candidate.address + ":" + e.candidate.port); };
	pc.onicecandidateerror = (e) => errors.push({ code: e.errorCode, text: e.errorText, url: e.url });
	pc.addTransceiver("audio");
	await pc.setLocalDescription(await pc.createOffer());
	await new Promise((done) => {
		const timer = setTimeout(done, 10000);
		pc.onicegatheringstatechange = () => { if (pc.iceGatheringState === "complete") { clearTimeout(timer); done(); } };
	});
	pc.close();
	return JSON.stringify({ relays, errors });
})`

type relayStat struct {
	Policy     string `json:"policy"`
	Local      string `json:"local"`
	Remote     string `json:"remote"`
	LocalAddr  string `json:"localAddr"`
	RemoteAddr string `json:"remoteAddr"`
	Bytes      int64  `json:"bytes"`
	Packets    int64  `json:"packets"`
}

type gatherResult struct {
	Relays []string `json:"relays"`
	Errors []struct {
		Code int    `json:"code"`
		Text string `json:"text"`
		URL  string `json:"url"`
	} `json:"errors"`
}

type turnServer struct {
	URLs     []string `json:"urls"`
	Username string   `json:"username"`
	Password string   `json:"credential"`
}

// turnCredential is a credential in coturn's REST scheme, as the server makes them:
// the user name begins with its expiry, and the password is the HMAC-SHA1 of the
// user name under the shared secret.
func turnCredential(secret string, expires time.Time) (user, password string) {
	user = fmt.Sprintf("%d:bingle", expires.Unix())
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(user))
	return user, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Two players talk through a real TURN relay, coturn in a container, with both
// browsers held to relay-only ICE: the call connects, the selected pair is relay
// to relay on both sides, and audio arrives. The credentials the server hands out
// are what the relay accepted; one whose time has passed, and one made with
// another secret, are refused.
//
// Opt-in (docs/test.md): BINGLE_TURN_RELAY=1, with podman or docker and a browser.
func TestVoiceUI_CallThroughTURNRelay(t *testing.T) {
	if os.Getenv("BINGLE_TURN_RELAY") == "" {
		t.Skip("set BINGLE_TURN_RELAY=1 to run the call through a coturn relay (needs podman or docker)")
	}
	if testing.Short() {
		t.Skip("browser tests are skipped with -short")
	}
	if findBrowser() == "" {
		t.Skip("no Chrome or Chromium found on $PATH (set BINGLE_BROWSER to its path)")
	}
	secret := randomHex(t, 24)
	relayURL := startCoturn(t, secret)

	cfg := newTestConfig(t)
	cfg.RTCTurnURLs = []string{relayURL}
	cfg.RTCTurnSecret = secret
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	baseURL := srv.URL

	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	alice, cancelA := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancelA()
	bob, cancelB := newChromedpContextWith(t, 120*time.Second, fakeMic...)
	defer cancelB()
	for _, ctx := range []context.Context{alice, bob} {
		if err := runSteps(ctx, stepAddScript(relayOnlyScript)); err != nil {
			t.Fatal(err)
		}
	}
	openGameAs(t, alice, baseURL, room, "tic-tac-toe", "Alice")
	openGameAs(t, bob, baseURL, room, "tic-tac-toe", "Bob")
	waitJS(t, alice, "the microphone in the chat", `document.getElementById("voice-join")`, 10*time.Second)
	waitJS(t, bob, "the microphone in the chat", `document.getElementById("voice-join")`, 10*time.Second)
	clickSel(t, alice, "#voice-join")
	waitJS(t, alice, "Alice in the call", `document.getElementById("voice-join").getAttribute("aria-pressed") === "true"`, 10*time.Second)
	clickSel(t, bob, "#voice-join")

	both := `document.querySelector('#voice-peers li[data-voice-seat="%d"]:not([data-me])') && document.querySelector('#voice-peers li[data-voice-seat="%d"]:not([data-me])').dataset.state === "connected"`
	waitJS(t, alice, "Alice connected to Bob", fmt.Sprintf(both, 2, 2), 40*time.Second)
	waitJS(t, bob, "Bob connected to Alice", fmt.Sprintf(both, 1, 1), 40*time.Second)

	// The selected pair is relay to relay on both sides, and audio keeps arriving.
	first := map[string]relayStat{}
	for name, ctx := range map[string]context.Context{"Alice": alice, "Bob": bob} {
		s := readRelayStat(t, ctx, name)
		if s.Policy != "relay" || s.Local != "relay" || s.Remote != "relay" {
			t.Fatalf("%s's selected pair: %+v, want relay-only ICE with relay candidates at both ends", name, s)
		}
		first[name] = s
	}
	if err := runSteps(alice, stepSleep(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	for name, ctx := range map[string]context.Context{"Alice": alice, "Bob": bob} {
		s := readRelayStat(t, ctx, name)
		f := first[name]
		t.Logf("%s: pair %s %s -> %s %s; audio received %d -> %d bytes, %d -> %d packets", name, s.Local, s.LocalAddr, s.Remote, s.RemoteAddr, f.Bytes, s.Bytes, f.Packets, s.Packets)
		if s.Bytes <= f.Bytes || s.Packets < f.Packets+50 {
			t.Errorf("%s's audio did not flow through the relay: %d -> %d bytes, %d -> %d packets in 3 s", name, f.Bytes, s.Bytes, f.Packets, s.Packets)
		}
	}

	// The server's own credential, gathered alone, gets a relay candidate; one
	// whose expiry has passed, and one made with another secret, are refused.
	var cfgOut struct {
		ICEServers []turnServer `json:"ice_servers"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/rtc/config", nil, &cfgOut); code != 200 || len(cfgOut.ICEServers) != 1 {
		t.Fatalf("rtc config: %d %+v", code, cfgOut)
	}
	served := cfgOut.ICEServers[0]
	if got := gatherWith(t, alice, served); len(got.Relays) == 0 || len(got.Errors) != 0 {
		t.Fatalf("the server's credential: relay candidates %v, errors %+v; want a relay candidate and no error", got.Relays, got.Errors)
	}
	expiredUser, expiredPass := turnCredential(secret, time.Now().Add(-time.Minute))
	wrongUser, wrongPass := turnCredential("not-"+secret, time.Now().Add(time.Hour))
	for _, c := range []struct {
		what string
		cred turnServer
	}{
		{"an expired credential", turnServer{URLs: []string{relayURL}, Username: expiredUser, Password: expiredPass}},
		{"a credential of another secret", turnServer{URLs: []string{relayURL}, Username: wrongUser, Password: wrongPass}},
	} {
		got := gatherWith(t, alice, c.cred)
		refused := false
		for _, e := range got.Errors {
			if e.Code == 401 {
				refused = true
			}
		}
		t.Logf("%s: relay candidates %v, errors %+v", c.what, got.Relays, got.Errors)
		if len(got.Relays) != 0 || !refused {
			t.Errorf("%s: relay candidates %v, errors %+v; want none, and a 401 from the relay", c.what, got.Relays, got.Errors)
		}
	}
}

func readRelayStat(t *testing.T, ctx context.Context, name string) relayStat {
	t.Helper()
	var raw string
	if err := runSteps(ctx, stepEvalTo(relayStatsJS, &raw, awaitPromise)); err != nil {
		t.Fatalf("%s's statistics: %v", name, err)
	}
	var stats []relayStat
	if err := json.Unmarshal([]byte(raw), &stats); err != nil || len(stats) != 1 {
		t.Fatalf("%s's statistics: %s (%v), want one connected peer", name, raw, err)
	}
	return stats[0]
}

func gatherWith(t *testing.T, ctx context.Context, s turnServer) gatherResult {
	t.Helper()
	servers, _ := json.Marshal([]map[string]any{{"urls": s.URLs, "username": s.Username, "credential": s.Password}})
	var raw string
	if err := runSteps(ctx, stepEvalTo(gatherJS+"("+string(servers)+")", &raw, awaitPromise)); err != nil {
		t.Fatalf("gathering candidates: %v", err)
	}
	var out gatherResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("gathering candidates: %s: %v", raw, err)
	}
	return out
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// containerTool is podman or docker, whichever is installed (podman first).
func containerTool() string {
	for _, name := range []string{"podman", "docker"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// startCoturn runs coturn in a container for the test and returns its turn: URL.
// It listens on 127.0.0.1 only, on a port of the test's own, and relays from a
// small range of ports beside it; the container has host networking, so the two
// browsers reach it on loopback and its relay addresses are loopback addresses.
// The container is removed when the test ends, whatever the result.
func startCoturn(t *testing.T, secret string) string {
	t.Helper()
	tool := containerTool()
	if tool == "" {
		t.Skip("neither podman nor docker is installed: the TURN test runs coturn in a container")
	}
	run := func(timeout time.Duration, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return exec.CommandContext(ctx, tool, args...).CombinedOutput()
	}
	if _, err := run(30*time.Second, "image", "exists", coturnImage); err != nil {
		if out, err := run(5*time.Minute, "pull", coturnImage); err != nil {
			t.Skipf("pulling %s failed (no network?): %v: %s", coturnImage, err, strings.TrimSpace(string(out)))
		}
	}
	listen, minRelay, maxRelay := freeTURNPorts(t)
	name := fmt.Sprintf("bingle-test-coturn-%d-%s", os.Getpid(), randomHex(t, 4))
	_, _ = run(30*time.Second, "rm", "-f", name)
	t.Cleanup(func() {
		if t.Failed() {
			if out, err := run(30*time.Second, "logs", name); err == nil {
				t.Logf("coturn's log:\n%s", out)
			}
		}
		if out, err := run(60*time.Second, "rm", "-f", name); err != nil {
			t.Errorf("removing the container %s: %v: %s", name, err, out)
		}
	})
	args := []string{"run", "-d", "--name", name, "--network=host", coturnImage,
		"-n", "--log-file=stdout", "--no-tls", "--fingerprint",
		"--listening-ip=127.0.0.1", "--relay-ip=127.0.0.1",
		fmt.Sprintf("--listening-port=%d", listen),
		fmt.Sprintf("--min-port=%d", minRelay), fmt.Sprintf("--max-port=%d", maxRelay),
		"--realm=bingle.test", "--use-auth-secret", "--static-auth-secret=" + secret,
		// Both browsers and the relay are on this machine, so every peer the
		// relay sends to is a loopback address, which coturn refuses unless told.
		// For the test only: a real relay must never allow loopback peers.
		"--allow-loopback-peers",
	}
	if out, err := run(60*time.Second, args...); err != nil {
		t.Fatalf("starting coturn: %v: %s", err, out)
	}
	// coturn listens on TCP as well as UDP on the port: it is up once it accepts.
	addr := fmt.Sprintf("127.0.0.1:%d", listen)
	deadline := time.Now().Add(30 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = c.Close()
			break
		}
		state, _ := run(30*time.Second, "inspect", "-f", "{{.State.Running}}", name)
		if strings.TrimSpace(string(state)) != "true" || time.Now().After(deadline) {
			t.Fatalf("coturn did not listen on %s: %v", addr, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Sprintf("turn:%s?transport=udp", addr)
}

// freeTURNPorts finds a listening port free on UDP and TCP and ten relay ports
// after it free on UDP, all on 127.0.0.1, in a window below the kernel's ephemeral
// range so that no outgoing connection is given one meanwhile. It never uses 3478,
// which a real relay on this machine may hold.
func freeTURNPorts(t *testing.T) (listen, minRelay, maxRelay int) {
	t.Helper()
	const base, size, relays = 24000, 2000, 10
	free := func(network string, port int) bool {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		if network == "udp" {
			c, err := net.ListenPacket("udp", addr)
			if err != nil {
				return false
			}
			_ = c.Close()
			return true
		}
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		_ = l.Close()
		return true
	}
	start := base + int(time.Now().UnixNano()%int64(size/(relays+1)))*(relays+1)
	for i := 0; i < size; i += relays + 1 {
		p := base + (start-base+i)%size
		if p+relays >= base+size || !free("udp", p) || !free("tcp", p) {
			continue
		}
		ok := true
		for r := p + 1; r <= p+relays && ok; r++ {
			ok = free("udp", r)
		}
		if ok {
			return p, p + 1, p + relays
		}
	}
	t.Skipf("no free ports for the relay between %d and %d", base, base+size)
	return 0, 0, 0
}
