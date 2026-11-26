package pack_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/tests/testdata"
)

// get asks the server for path with the headers given, and returns the
// answer with its body read.
func (s *server) get(path string, header ...string) (*http.Response, []byte) {
	s.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, s.url+"/api"+path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("GET %s: %v", path, err)
	}
	return resp, body
}

type clientIndex struct {
	Game   string `json:"game"`
	Client string `json:"client"`
	Files  []struct {
		Path        string `json:"path"`
		Size        int64  `json:"size"`
		SHA256      string `json:"sha256"`
		ContentType string `json:"content_type"`
	} `json:"files"`
}

// What a client with nothing but the API does: find the games with its
// files, list them, fetch each it lacks, check it against its hash, and keep
// it by its hash. The second time, it fetches nothing.
func TestAClientFetchesAPackGamesFilesOnce(t *testing.T) {
	packs, dir := testdata.DraughtsPack(t)
	s := start(t, filepath.Join(t.TempDir(), "packs.db"), packs)

	var list []struct {
		ID      string   `json:"id"`
		Clients []string `json:"clients"`
	}
	s.must(http.MethodGet, "/game-defs", nil, &list)
	var games []string
	for _, g := range list {
		if slices.Contains(g.Clients, madeUp) {
			games = append(games, g.ID)
		}
	}
	// The pack's game and its variant, and no built-in game.
	if want := []string{testdata.DraughtsPackID, testdata.DraughtsPackSmallID}; !slices.Equal(games, want) {
		t.Fatalf("the games with %s's files are %v, want %v", madeUp, games, want)
	}

	kept := map[string][]byte{} // by hash
	fetched := 0
	fetch := func(game string) {
		t.Helper()
		var info struct {
			Clients []string `json:"clients"`
		}
		s.must(http.MethodGet, "/game-defs/"+game, nil, &info)
		if !slices.Equal(info.Clients, []string{madeUp}) {
			t.Errorf("%s's info names the clients %v", game, info.Clients)
		}
		var index clientIndex
		s.must(http.MethodGet, "/game-defs/"+game+"/clients/"+madeUp, nil, &index)
		if index.Game != game || index.Client != madeUp || len(index.Files) != len(madeUpFiles) {
			t.Fatalf("the index of %s: %+v", game, index)
		}
		for i, f := range index.Files {
			at := filepath.Join("clients", madeUp, f.Path)
			if want := madeUpFiles[i]; f.Path != want.path || f.ContentType != want.contentType || f.SHA256 != hashOf(t, dir, at) {
				t.Errorf("listed %+v, want %s (%s, %s)", f, want.path, want.contentType, hashOf(t, dir, at))
			}
			url := "/game-defs/" + game + "/clients/" + madeUp + "/files/" + f.Path
			var header []string
			if _, ok := kept[f.SHA256]; ok {
				header = []string{"If-None-Match", `"` + f.SHA256 + `"`}
			}
			resp, body := s.get(url, header...)
			switch resp.StatusCode {
			case http.StatusNotModified:
				if len(body) != 0 {
					t.Errorf("%s: a body with 304", f.Path)
				}
				continue
			case http.StatusOK:
			default:
				t.Fatalf("GET %s = %d", url, resp.StatusCode)
			}
			fetched++
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != f.SHA256 || int64(len(body)) != f.Size {
				t.Errorf("%s does not match its hash or size", f.Path)
			}
			for name, want := range map[string]string{
				"ETag":                   `"` + f.SHA256 + `"`,
				"Content-Type":           f.ContentType,
				"X-Content-Type-Options": "nosniff",
				"Content-Disposition":    `attachment; filename=` + filepath.Base(f.Path),
			} {
				if got := resp.Header.Get(name); got != want {
					t.Errorf("%s: %s is %q, want %q", f.Path, name, got, want)
				}
			}
			if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
				t.Errorf("%s is not sandboxed", f.Path)
			}
			kept[f.SHA256] = body
		}
	}
	for _, game := range games {
		fetch(game)
	}
	// The variant's files are the same files.
	if fetched != len(madeUpFiles) {
		t.Errorf("fetched %d files for two games, want %d", fetched, len(madeUpFiles))
	}
	fetched = 0
	for _, game := range games {
		fetch(game)
	}
	if fetched != 0 {
		t.Errorf("fetched %d files the second time", fetched)
	}

	// A range of a file, as a download that is resumed asks for one.
	look := "/game-defs/" + testdata.DraughtsPackID + "/clients/" + madeUp + "/files/look.toml"
	whole := kept[hashOf(t, dir, "clients/made-up/look.toml")]
	resp, body := s.get(look, "Range", "bytes=4-11")
	if resp.StatusCode != http.StatusPartialContent || string(body) != string(whole[4:12]) {
		t.Errorf("a range: %d %q, want 206 %q", resp.StatusCode, body, whole[4:12])
	}
	// One whose file has changed since is sent whole.
	resp, body = s.get(look, "Range", "bytes=4-11", "If-Range", `"0000"`)
	if resp.StatusCode != http.StatusOK || len(body) != len(whole) {
		t.Errorf("a range of another version: %d, %d bytes", resp.StatusCode, len(body))
	}
}

func TestClientFilesThatAreNotThereAnswer404(t *testing.T) {
	packs, _ := testdata.DraughtsPack(t)
	s := start(t, filepath.Join(t.TempDir(), "packs.db"), packs)
	for _, path := range []string{
		"/game-defs/" + testdata.DraughtsPackID + "/clients/bingle-3d",
		"/game-defs/" + testdata.DraughtsPackID + "/clients/bingle-3d/files/look.toml",
		"/game-defs/" + testdata.DraughtsPackID + "/clients/" + madeUp + "/files/nothing.toml",
		"/game-defs/" + testdata.DraughtsPackID + "/clients/" + madeUp + "/files/models",
		"/game-defs/" + testdata.DraughtsPackID + "/clients/" + madeUp + "/files/%2e%2e/%2e%2e/manifest.yaml",
		"/game-defs/" + testdata.DraughtsPackID + "/clients/" + madeUp + "/files/..%2f..%2frules.wasm",
		"/game-defs/draughts/clients/" + madeUp,
		"/game-defs/draughts/clients/" + madeUp + "/files/look.toml",
		"/game-defs/no-such-game/clients/" + madeUp,
	} {
		if resp, _ := s.get(path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
	// A built-in game has no clients.
	_, raw := s.get("/game-defs/draughts")
	var info map[string]any
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if _, has := info["clients"]; has {
		t.Errorf("a built-in game's info names clients: %v", info["clients"])
	}
}

// A file changed on the disk after its pack loaded is not sent under the
// hash of the one that was.
func TestAClientFileChangedOnTheDiskIsNotSent(t *testing.T) {
	packs, dir := testdata.DraughtsPack(t)
	s := start(t, filepath.Join(t.TempDir(), "packs.db"), packs)
	write(t, dir, "clients/made-up/look.toml", "board = \"elsewhere.png\"\n")
	resp, body := s.get("/game-defs/" + testdata.DraughtsPackID + "/clients/" + madeUp + "/files/look.toml")
	if resp.StatusCode != http.StatusInternalServerError || strings.Contains(string(body), "elsewhere") {
		t.Errorf("a changed file: %d %s", resp.StatusCode, body)
	}
}

// A large file goes from the disk to the client without being read into
// the server's memory.
func TestALargeClientFileIsNotHeldInMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("writes and hashes 48 MiB")
	}
	packs, dir := testdata.DraughtsPack(t)
	const size = 48 << 20
	glb(t, dir, "clients/made-up/models/board.glb", size)
	s := start(t, filepath.Join(t.TempDir(), "packs.db"), packs)

	var before, after goruntime.MemStats
	goruntime.ReadMemStats(&before)
	req, _ := http.NewRequest(http.MethodGet, s.url+"/api/game-defs/"+testdata.DraughtsPackID+"/clients/"+madeUp+"/files/models/board.glb", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	n, err := io.Copy(h, resp.Body)
	_ = resp.Body.Close()
	goruntime.ReadMemStats(&after)
	if err != nil || n != size {
		t.Fatalf("read %d bytes: %v", n, err)
	}
	if hex.EncodeToString(h.Sum(nil)) != hashOf(t, dir, "clients/made-up/models/board.glb") {
		t.Error("the file does not match its hash")
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated > size/4 {
		t.Errorf("sending %d bytes allocated %d", size, allocated)
	}
	t.Logf("sending %d bytes allocated %d, in the server and the test together", size, allocated)
}
