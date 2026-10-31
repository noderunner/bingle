package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/config"
)

const usage = `usage:
  bingle                                    run the server
  bingle account list                       list the accounts
  bingle account create HANDLE EMAIL [--admin]
                                            make a confirmed account; prints its password,
                                            which must be changed at the first sign-in
  bingle account reset HANDLE               give an account a new password (printed), to be
                                            changed at the next sign-in, and sign it out everywhere
  bingle caps reset                         apply the capabilities file (BINGLE_CAPS_FILE) again
`

// runCommand runs one command line and returns the exit status.
func runCommand(cfg config.Config, args []string) int {
	return command(cfg, args, os.Stdout, os.Stderr)
}

func command(cfg config.Config, args []string, out, errOut io.Writer) int {
	if len(args) < 2 || (args[0] != "account" && args[0] != "caps") {
		_, _ = fmt.Fprint(errOut, usage)
		return 2
	}
	a, err := app.New(cfg)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "bingle:", err)
		return 1
	}
	defer func() { _ = a.Close() }()
	printSeededTo(errOut, a.Seeded())
	ctx := context.Background()
	acc := a.Accounts()
	if args[0] == "caps" {
		if args[1] != "reset" || len(args) != 2 {
			_, _ = fmt.Fprint(errOut, usage)
			return 2
		}
		if err := a.Caps().Reset(ctx); err != nil {
			_, _ = fmt.Fprintln(errOut, "bingle:", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "the capabilities are those of %s again\n", capsSource(cfg.CapsFile))
		return 0
	}
	switch args[1] {
	case "list":
		list, err := a.ListAccounts(ctx)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, "bingle:", err)
			return 1
		}
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "HANDLE\tSTATUS\tGROUPS\tCREATED")
		for _, l := range list {
			_, _ = fmt.Fprintf(tw, "@%s\t%s\t%s\t%s\n", l.Handle, l.Status, strings.Join(l.Groups, ","), l.CreatedAt)
		}
		_ = tw.Flush()
		return 0
	case "create":
		rest := args[2:]
		admin := false
		var pos []string
		for _, r := range rest {
			if r == "--admin" {
				admin = true
			} else {
				pos = append(pos, r)
			}
		}
		if len(pos) != 2 {
			_, _ = fmt.Fprint(errOut, usage)
			return 2
		}
		pw, err := acc.Create(ctx, pos[0], pos[1], admin)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, "bingle:", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "@%s can sign in with the password %s (and must change it then)\n", strings.ToLower(strings.TrimPrefix(pos[0], "@")), pw)
		return 0
	case "reset":
		if len(args) != 3 {
			_, _ = fmt.Fprint(errOut, usage)
			return 2
		}
		pw, err := acc.ResetPassword(ctx, strings.ToLower(strings.TrimPrefix(args[2], "@")))
		if err != nil {
			_, _ = fmt.Fprintln(errOut, "bingle:", err)
			return 1
		}
		_, _ = fmt.Fprintf(out, "@%s can sign in with the password %s (and must change it then); every session was signed out\n", strings.ToLower(strings.TrimPrefix(args[2], "@")), pw)
		return 0
	}
	_, _ = fmt.Fprint(errOut, usage)
	return 2
}

// printSeeded tells the operator, once, how to sign in to the administrator
// made for an empty database. It goes to the terminal, never to the log: a
// password is not a log line (ADR 0063).
func printSeeded(s *auth.Seeded) { printSeededTo(os.Stderr, s) }

func printSeededTo(w io.Writer, s *auth.Seeded) {
	if s == nil {
		return
	}
	if s.Deterministic {
		_, _ = fmt.Fprintf(w, "\nbingle: made the administrator @%s with the development password %q (this server is on loopback or BINGLE_DEV); it must be changed at the first sign-in.\n\n", s.Handle, s.Password)
		return
	}
	_, _ = fmt.Fprintf(w, "\nbingle: made the administrator @%s. Its password, shown only now, is:\n\n    %s\n\nSign in at /login; you will be asked to change it.\n\n", s.Handle, s.Password)
}

func capsSource(file string) string {
	if file == "" {
		return "the built-in defaults"
	}
	if _, err := os.Stat(file); err != nil {
		return "the built-in defaults (there is no " + file + ")"
	}
	return file
}
