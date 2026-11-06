// Command ghapp acts on the repository as bingle's GitHub App, for agent work (CLAUDE.md).
//
//	go run ./scripts/ghapp token                    print an installation token (for a git credential helper)
//	go run ./scripts/ghapp api METHOD PATH [<body]  call the REST API as the app; the token never leaves the process
//	go run ./scripts/ghapp push [-force] BRANCH     recreate the local branch's commits on GitHub, signed by GitHub
//
// push exists because a GitHub App cannot sign a commit it pushes with git: only commits it makes
// through the API are signed and verified, and `main` takes only signed commits. Each local commit
// on BRANCH since `main` is sent with GraphQL's createCommitOnBranch, its tree is checked against the
// local one, and the local branch is moved to the commits GitHub made.
//
// It reads BINGLE_GH_APP_ID (the app's client ID or app ID), BINGLE_GH_APP_KEY (the path to the app's
// private key) and BINGLE_GH_REMOTE (the repository's HTTPS URL).
package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// apiBase is GitHub's API; a test points it elsewhere.
var apiBase = "https://api.github.com"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "ghapp:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: ghapp token | api METHOD PATH | push [-force] BRANCH")
	}
	owner, repo, err := repoOf(os.Getenv("BINGLE_GH_REMOTE"))
	if err != nil {
		return err
	}
	switch args[0] {
	case "token":
		tok, err := installationToken(owner, repo)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, tok)
		return err
	case "api":
		if len(args) != 3 {
			return errors.New("usage: ghapp api METHOD PATH [<body]")
		}
		tok, err := installationToken(owner, repo)
		if err != nil {
			return err
		}
		var body []byte
		if args[1] != http.MethodGet && args[1] != http.MethodDelete {
			if body, err = io.ReadAll(stdin); err != nil {
				return err
			}
		}
		out, status, err := call(tok, args[1], args[2], body)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(out); err != nil {
			return err
		}
		if status >= 300 {
			return fmt.Errorf("%s %s: HTTP %d", args[1], args[2], status)
		}
		return nil
	case "push":
		fs := flag.NewFlagSet("push", flag.ContinueOnError)
		force := fs.Bool("force", false, "start the remote branch again from main (after a rebase or an amend)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: ghapp push [-force] BRANCH")
		}
		tok, err := installationToken(owner, repo)
		if err != nil {
			return err
		}
		return push(tok, owner, repo, fs.Arg(0), *force, stdout)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

// repoOf reads the owner and name out of a repository's HTTPS URL.
func repoOf(remote string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(remote))
	if err != nil || u.Host == "" {
		return "", "", errors.New("BINGLE_GH_REMOTE is not set to the repository's HTTPS URL")
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("BINGLE_GH_REMOTE %q does not name owner/repo", remote)
	}
	return parts[0], parts[1], nil
}

// appJWT is the app's own credential, good for ten minutes at most: RS256, iat a minute back.
func appJWT(issuer string, key *rsa.PrivateKey, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": issuer,
	})
	if err != nil {
		return "", err
	}
	signed := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signed + "." + enc.EncodeToString(sig), nil
}

func readKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return nil, errors.New("BINGLE_GH_APP_KEY is not set to the path of the app's private key")
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own setting
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("BINGLE_GH_APP_KEY is not a PEM file")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("BINGLE_GH_APP_KEY: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("BINGLE_GH_APP_KEY is not an RSA key")
	}
	return rk, nil
}

// installationToken trades the app's JWT for an hour's token on the repository's installation.
func installationToken(owner, repo string) (string, error) {
	issuer := os.Getenv("BINGLE_GH_APP_ID")
	if issuer == "" {
		return "", errors.New("BINGLE_GH_APP_ID is not set to the app's client ID")
	}
	key, err := readKey(os.Getenv("BINGLE_GH_APP_KEY"))
	if err != nil {
		return "", err
	}
	jwt, err := appJWT(issuer, key, time.Now())
	if err != nil {
		return "", err
	}
	var inst struct {
		ID int64 `json:"id"`
	}
	if err := callJSON(jwt, http.MethodGet, "/repos/"+owner+"/"+repo+"/installation", nil, &inst); err != nil {
		return "", fmt.Errorf("the app is not installed on %s/%s: %w", owner, repo, err)
	}
	var tok struct {
		Token string `json:"token"`
	}
	path := "/app/installations/" + strconv.FormatInt(inst.ID, 10) + "/access_tokens"
	if err := callJSON(jwt, http.MethodPost, path, map[string]any{"repositories": []string{repo}}, &tok); err != nil {
		return "", err
	}
	if tok.Token == "" {
		return "", errors.New("GitHub gave no installation token")
	}
	return tok.Token, nil
}

// call sends one request to the API and returns the body and status.
func call(bearer, method, path string, body []byte) ([]byte, int, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequest(method, apiBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	return out, resp.StatusCode, err
}

// callJSON is call with a JSON body and answer; a status of 300 or more is an error.
func callJSON(bearer, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	raw, status, err := call(bearer, method, path, body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, status, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// fileChanges is createCommitOnBranch's description of a commit: what it adds and deletes.
type fileChanges struct {
	Additions []addition `json:"additions"`
	Deletions []deletion `json:"deletions"`
}

type addition struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
}

type deletion struct {
	Path string `json:"path"`
}

// changesOf reads what commit did to its parent. createCommitOnBranch sends contents, not modes, so a
// commit that adds a file other than an ordinary one, changes a mode, or touches a link or a
// submodule is refused here rather than recreated wrongly. (push also compares the trees.)
func changesOf(commit string) (fileChanges, error) {
	raw, err := exec.Command("git", "diff-tree", "-r", "-z", "--no-renames", "--no-commit-id", commit+"^", commit).Output()
	if err != nil {
		return fileChanges{}, fmt.Errorf("git diff-tree %s: %w", commit, err)
	}
	ch := fileChanges{Additions: []addition{}, Deletions: []deletion{}}
	fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		// ":old-mode new-mode old-sha new-sha status", then the path.
		meta, path := strings.Fields(strings.TrimPrefix(fields[i], ":")), fields[i+1]
		if len(meta) != 5 {
			return fileChanges{}, fmt.Errorf("%s: cannot read %q", commit, fields[i])
		}
		oldMode, newMode, status := meta[0], meta[1], meta[4]
		if status == "D" {
			ch.Deletions = append(ch.Deletions, deletion{Path: path})
			continue
		}
		added := oldMode == "000000"
		if (added && newMode != "100644") || (!added && oldMode != newMode) || newMode == "120000" || newMode == "160000" {
			return fileChanges{}, fmt.Errorf("%s: %s has mode %s (was %s); only ordinary files can be sent, so push this one by hand", commit, path, newMode, oldMode)
		}
		content, err := exec.Command("git", "show", commit+":"+path).Output()
		if err != nil {
			return fileChanges{}, fmt.Errorf("git show %s:%s: %w", commit, path, err)
		}
		ch.Additions = append(ch.Additions, addition{Path: path, Contents: base64.StdEncoding.EncodeToString(content)})
	}
	return ch, nil
}

const createCommit = `mutation($input: CreateCommitOnBranchInput!) {
  createCommitOnBranch(input: $input) { commit { oid } }
}`

// push recreates branch's commits since main on GitHub, one createCommitOnBranch each.
func push(tok, owner, repo, branch string, force bool, stdout io.Writer) error {
	refs := "/repos/" + owner + "/" + repo + "/git/refs"
	var mainRef struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := callJSON(tok, http.MethodGet, "/repos/"+owner+"/"+repo+"/git/ref/heads/main", nil, &mainRef); err != nil {
		return err
	}
	if _, err := git("cat-file", "-e", mainRef.Object.SHA+"^{commit}"); err != nil {
		return fmt.Errorf("main on GitHub is at %s, which this clone does not have: fetch it first", mainRef.Object.SHA)
	}
	if _, err := git("rev-parse", "--verify", branch+"^{commit}"); err != nil {
		return err
	}
	base, err := git("merge-base", branch, mainRef.Object.SHA)
	if err != nil {
		return err
	}

	// Where the remote branch is: absent, an ancestor of ours, or somewhere else (only with -force).
	head := ""
	raw, status, err := call(tok, http.MethodGet, "/repos/"+owner+"/"+repo+"/git/ref/heads/"+branch, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		var r struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		head = r.Object.SHA
		if _, err := git("merge-base", "--is-ancestor", head, branch); err != nil {
			if !force {
				return fmt.Errorf("%s on GitHub (%s) is not part of the local branch; after a rebase or an amend, push with -force", branch, head)
			}
			if err := callJSON(tok, http.MethodPatch, refs+"/heads/"+branch, map[string]any{"sha": base, "force": true}, nil); err != nil {
				return err
			}
			head = base
		}
	case http.StatusNotFound:
		if err := callJSON(tok, http.MethodPost, refs, map[string]any{"ref": "refs/heads/" + branch, "sha": base}, nil); err != nil {
			return err
		}
		head = base
	default:
		return fmt.Errorf("GET the branch: HTTP %d: %s", status, raw)
	}

	list, err := git("rev-list", "--reverse", "--topo-order", head+".."+branch)
	if err != nil {
		return err
	}
	if list == "" {
		_, err := fmt.Fprintf(stdout, "%s is up to date on GitHub at %s\n", branch, head)
		return err
	}
	commits := strings.Fields(list)
	for _, c := range commits {
		if parents, _ := git("rev-list", "--parents", "-n1", c); len(strings.Fields(parents)) != 2 {
			return fmt.Errorf("%s is a merge commit; rebase the branch onto main instead", c)
		}
	}
	for _, c := range commits {
		changes, err := changesOf(c)
		if err != nil {
			return err
		}
		msg, err := git("log", "-1", "--format=%B", c)
		if err != nil {
			return err
		}
		headline, body, _ := strings.Cut(msg, "\n")
		input := map[string]any{
			"branch":          map[string]any{"repositoryNameWithOwner": owner + "/" + repo, "branchName": branch},
			"message":         map[string]any{"headline": headline, "body": strings.TrimSpace(body)},
			"expectedHeadOid": head,
			"fileChanges":     changes,
		}
		var res struct {
			Data struct {
				CreateCommitOnBranch struct {
					Commit struct {
						OID string `json:"oid"`
					} `json:"commit"`
				} `json:"createCommitOnBranch"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := callJSON(tok, http.MethodPost, "/graphql", map[string]any{"query": createCommit, "variables": map[string]any{"input": input}}, &res); err != nil {
			return err
		}
		if len(res.Errors) > 0 {
			return fmt.Errorf("%s: %s", c, res.Errors[0].Message)
		}
		made := res.Data.CreateCommitOnBranch.Commit.OID
		if made == "" {
			return fmt.Errorf("%s: GitHub made no commit", c)
		}
		// The commit GitHub made must hold exactly the local commit's files.
		var remote struct {
			Tree struct {
				SHA string `json:"sha"`
			} `json:"tree"`
		}
		if err := callJSON(tok, http.MethodGet, "/repos/"+owner+"/"+repo+"/git/commits/"+made, nil, &remote); err != nil {
			return err
		}
		want, err := git("rev-parse", c+"^{tree}")
		if err != nil {
			return err
		}
		if remote.Tree.SHA != want {
			return fmt.Errorf("%s: GitHub's commit %s has tree %s, the local one %s; stop and compare by hand", c, made, remote.Tree.SHA, want)
		}
		if _, err := fmt.Fprintf(stdout, "%s -> %s  %s\n", c[:12], made[:12], headline); err != nil {
			return err
		}
		head = made
	}
	_, err = fmt.Fprintf(stdout, "%s is at %s on GitHub. Fetch it and point the local branch there:\n"+
		"  git fetch <remote> %s && git update-ref refs/heads/%s FETCH_HEAD %s\n", branch, head, branch, branch, commits[len(commits)-1])
	return err
}
