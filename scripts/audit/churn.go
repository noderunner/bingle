package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// churn: commits touching each file in the last 30 and 90 days, beside its
// size. Each pull request is squashed into one commit on main, so a commit is
// roughly a pull request.
func (a *audit) churn() error {
	ms, err := a.measureAll()
	if err != nil {
		return err
	}
	c30, err := commitsSince("30.days")
	if err != nil {
		return err
	}
	c90, err := commitsSince("90.days")
	if err != nil {
		return err
	}
	var rows [][]string
	groups := map[string][3]int{}
	for _, m := range ms {
		rows = append(rows, []string{m.path, kind(m.test), itoa(m.lines), itoa(c30[m.path]), itoa(c90[m.path]), itoa(m.lines * c30[m.path] / 100)})
		if !m.test {
			g := groups[group(m.path)]
			groups[group(m.path)] = [3]int{g[0] + m.lines, g[1] + c30[m.path], g[2] + c90[m.path]}
		}
	}
	head := []string{"file", "kind", "lines", "commits 30d", "commits 90d", "lines × commits 30d / 100"}
	byInt(rows, 5)
	var code [][]string
	for _, r := range rows {
		if r[1] == "code" {
			code = append(code, r)
		}
	}
	fmt.Println("\nCode files by size times churn (last 30 days):")
	table(head, code, a.top)
	var g [][]string
	for k, v := range groups {
		g = append(g, []string{k, itoa(v[0]), itoa(v[1]), itoa(v[2])})
	}
	byInt(g, 2)
	ghead := []string{"group", "lines", "file-commits 30d", "file-commits 90d"}
	fmt.Println("\nGroups by file-commits in the last 30 days (code only):")
	table(ghead, g, a.top)
	if err := a.save("churn.tsv", head, rows); err != nil {
		return err
	}
	return a.save("churn-groups.tsv", ghead, g)
}

func commitsSince(since string) (map[string]int, error) {
	out, err := exec.Command("git", "log", "--since="+since, "--format=", "--name-only", "--no-renames", "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	n := map[string]int{}
	for _, l := range strings.Split(string(out), "\n") {
		if l != "" {
			n[l]++
		}
	}
	return n, nil
}
