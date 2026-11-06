package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepoOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r":      "o/r",
		"https://github.com/o/r.git":  "o/r",
		"https://github.com/o/r.git/": "o/r",
	} {
		o, r, err := repoOf(in)
		if err != nil || o+"/"+r != want {
			t.Errorf("repoOf(%q) = %q, %q, %v", in, o, r, err)
		}
	}
	for _, in := range []string{"", "github.com/o/r", "https://github.com/o", "https://github.com/o/r/x"} {
		if _, _, err := repoOf(in); err == nil {
			t.Errorf("repoOf(%q) took it", in)
		}
	}
}

func TestAppJWTIsSignedAndShortLived(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	tok, err := appJWT("Iv23abc", key, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("%d parts", len(parts))
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("signature: %v", err)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iat, Exp int64
		Iss      string
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if c.Iss != "Iv23abc" || c.Iat >= now.Unix() || c.Exp > now.Add(10*time.Minute).Unix() || c.Exp <= now.Unix() {
		t.Errorf("claims %+v", c)
	}
}

// gitRepo makes a repository in a temporary directory and works in it.
func gitRepo(t *testing.T) func(args ...string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	g := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	g("init", "-q", "-b", "main")
	return g
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestChangesOfReadsAdditionsAndDeletionsAndRefusesModes(t *testing.T) {
	g := gitRepo(t)
	write(t, "a.txt", "one\n", 0o644)
	write(t, "gone.txt", "bye\n", 0o644)
	write(t, "run.sh", "#!/bin/sh\n", 0o755)
	g("add", ".")
	g("commit", "-q", "-m", "start")
	write(t, "a.txt", "two\n", 0o644)
	write(t, "dir/new.bin", "\x00\x01", 0o644)
	write(t, "run.sh", "#!/bin/sh\necho\n", 0o755) // an executable edited, keeping its mode
	g("rm", "-q", "gone.txt")
	g("add", ".")
	g("commit", "-q", "-m", "change")

	ch, err := changesOf("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, a := range ch.Additions {
		b, _ := base64.StdEncoding.DecodeString(a.Contents)
		got[a.Path] = string(b)
	}
	if len(got) != 3 || got["a.txt"] != "two\n" || got["dir/new.bin"] != "\x00\x01" || got["run.sh"] != "#!/bin/sh\necho\n" {
		t.Errorf("additions %q", got)
	}
	if len(ch.Deletions) != 1 || ch.Deletions[0].Path != "gone.txt" {
		t.Errorf("deletions %+v", ch.Deletions)
	}

	write(t, "tool.sh", "#!/bin/sh\n", 0o755)
	g("add", ".")
	g("commit", "-q", "-m", "a new executable")
	if _, err := changesOf("HEAD"); err == nil || !strings.Contains(err.Error(), "tool.sh") {
		t.Errorf("a new executable: %v", err)
	}
	g("update-index", "--chmod=-x", "run.sh")
	g("commit", "-q", "-m", "a mode change")
	if _, err := changesOf("HEAD"); err == nil || !strings.Contains(err.Error(), "run.sh") {
		t.Errorf("a mode change: %v", err)
	}
}

// fakeGitHub answers the calls push makes. Each commit it is sent "becomes" the next local commit,
// whose tree it reports, unless bad says to report another.
type fakeGitHub struct {
	t       *testing.T
	main    string
	branch  string // the remote branch's head, "" when it does not exist
	local   []string
	made    int
	bad     bool
	headOID []string // expectedHeadOid of each commit
	heads   []string // the headline of each commit
	reset   string   // where a forced PATCH moved the branch
	tmp     []string // the branches made and deleted besides it
	deleted []string
	onto    []string // the branch each commit was made on
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == "GET" && r.URL.Path == "/repos/o/r/git/ref/heads/main":
		_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": f.main}})
	case r.Method == "GET" && r.URL.Path == "/repos/o/r/git/ref/heads/topic":
		if f.branch == "" {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": f.branch}})
	case r.Method == "POST" && r.URL.Path == "/repos/o/r/git/refs":
		var in struct{ Ref, SHA string }
		_ = json.Unmarshal(body, &in)
		if in.Ref == "refs/heads/topic" {
			f.branch = in.SHA
		} else {
			f.tmp = append(f.tmp, in.Ref)
		}
		w.WriteHeader(201)
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/repos/o/r/git/refs/heads/ghapp-tmp/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/repos/o/r/git/refs/"))
		w.WriteHeader(204)
	case r.Method == "PATCH" && r.URL.Path == "/repos/o/r/git/refs/heads/topic":
		var in struct {
			SHA   string
			Force bool
		}
		_ = json.Unmarshal(body, &in)
		if !in.Force {
			f.t.Error("PATCH without force")
		}
		f.reset, f.branch = in.SHA, in.SHA
	case r.Method == "POST" && r.URL.Path == "/graphql":
		var in struct {
			Variables struct {
				Input struct {
					ExpectedHeadOid string
					Message         struct{ Headline string }
					Branch          struct{ BranchName string }
				}
			}
		}
		_ = json.Unmarshal(body, &in)
		f.headOID = append(f.headOID, in.Variables.Input.ExpectedHeadOid)
		f.heads = append(f.heads, in.Variables.Input.Message.Headline)
		f.onto = append(f.onto, in.Variables.Input.Branch.BranchName)
		oid := strings.Repeat("ab", 19) + string(rune('0'+f.made))
		f.made++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"createCommitOnBranch": map[string]any{"commit": map[string]string{"oid": oid}}}})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/o/r/git/commits/"):
		tree, _ := exec.Command("git", "rev-parse", f.local[f.made-1]+"^{tree}").Output()
		sha := strings.TrimSpace(string(tree))
		if f.bad {
			sha = strings.Repeat("0", 40)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tree": map[string]string{"sha": sha}})
	default:
		f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}
}

func TestPushRecreatesEachCommitInOrderOnTopOfMain(t *testing.T) {
	g := gitRepo(t)
	write(t, "a.txt", "1\n", 0o644)
	g("add", ".")
	g("commit", "-q", "-m", "main")
	main := g("rev-parse", "HEAD")
	g("checkout", "-q", "-b", "topic")
	var local []string
	for i, msg := range []string{"First change\n\nWith a body.", "Second change"} {
		write(t, "a.txt", strings.Repeat("x", i+2)+"\n", 0o644)
		g("commit", "-q", "-am", msg)
		local = append(local, g("rev-parse", "HEAD"))
	}

	f := &fakeGitHub{t: t, main: main, local: local}
	srv := httptest.NewServer(f)
	defer srv.Close()
	apiBase = srv.URL

	var out bytes.Buffer
	if err := push("tok", "o", "r", "topic", false, &out); err != nil {
		t.Fatalf("push: %v\n%s", err, out.String())
	}
	if f.made != 2 || f.heads[0] != "First change" || f.heads[1] != "Second change" {
		t.Fatalf("made %d: %q", f.made, f.heads)
	}
	if f.headOID[0] != main || f.headOID[1] != strings.Repeat("ab", 19)+"0" {
		t.Errorf("each commit goes on the one before: %q", f.headOID)
	}
	if !strings.Contains(out.String(), "git update-ref refs/heads/topic FETCH_HEAD "+local[1]) {
		t.Errorf("no way back to the local branch:\n%s", out.String())
	}

	// A branch that has moved on GitHub is not replaced without -force; with it, it starts from main again.
	f2 := &fakeGitHub{t: t, main: main, local: local, branch: strings.Repeat("cd", 20)}
	srv2 := httptest.NewServer(f2)
	defer srv2.Close()
	apiBase = srv2.URL
	if err := push("tok", "o", "r", "topic", false, io.Discard); err == nil || !strings.Contains(err.Error(), "-force") {
		t.Fatalf("a diverged branch without -force: %v", err)
	}
	if err := push("tok", "o", "r", "topic", true, io.Discard); err != nil || f2.made != 2 {
		t.Fatalf("with -force: %v, made %d", err, f2.made)
	}
	// The pull request's branch never points at main, which would close it: the commits are made on a
	// branch of their own, the branch moves once to the last of them, and the other is deleted.
	if f2.reset != strings.Repeat("ab", 19)+"1" {
		t.Errorf("with -force the branch was moved to %q, want the last commit made", f2.reset)
	}
	if len(f2.tmp) != 1 || len(f2.deleted) != 1 || "refs/"+f2.deleted[0] != f2.tmp[0] {
		t.Errorf("the branch of its own: made %q, deleted %q", f2.tmp, f2.deleted)
	}
	for _, b := range f2.onto {
		if b == "topic" {
			t.Errorf("a commit was made on the pull request's branch itself: %q", f2.onto)
		}
	}

	// A commit GitHub made with other files stops the push.
	f3 := &fakeGitHub{t: t, main: main, local: local, bad: true}
	srv3 := httptest.NewServer(f3)
	defer srv3.Close()
	apiBase = srv3.URL
	if err := push("tok", "o", "r", "topic", false, io.Discard); err == nil || !strings.Contains(err.Error(), "compare by hand") || f3.made != 1 {
		t.Fatalf("a wrong tree: %v, made %d", err, f3.made)
	}
}
