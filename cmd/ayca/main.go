// Command ayca — All Your Context Are Belong To Us. Know what your agents know.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/R037/ayca/internal/budget"
	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/preview"
	"github.com/R037/ayca/internal/tree"
	"github.com/R037/ayca/internal/tui"
)

var version = "0.1.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ayca", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("path", "", "project directory to inspect (default: current directory)")
	agentFlag := fs.String("agent", "", "only show context for these agents, comma-separated: "+strings.Join(discovery.AllAgents, ","))
	window := fs.String("window", "", "context window size to compare against, e.g. 200k or 1m (default: per agent — claude 200k, codex 272k, cursor 200k)")
	list := fs.Bool("list", false, "print discovered context as plain text and exit (no TUI)")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "AYCA — All Your Context Are Belong To Us. Know what your agents know.\n\nUsage: ayca [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "ayca", version)
		return 0
	}

	agents, err := parseAgents(*agentFlag)
	if err != nil {
		fmt.Fprintln(stderr, "ayca:", err)
		return 2
	}
	var windows map[string]int
	if *window != "" {
		w, err := budget.ParseWindow(*window)
		if err != nil {
			fmt.Fprintln(stderr, "ayca:", err)
			return 2
		}
		windows = map[string]int{}
		for _, a := range discovery.AllAgents {
			windows[a] = w
		}
	}
	env, err := discovery.NewEnv(*path)
	if err != nil {
		fmt.Fprintln(stderr, "ayca:", err)
		return 1
	}
	res := discovery.Discover(env, agents)

	if *list {
		printList(stdout, res)
		printBudget(stdout, budget.Estimate(res, agents, windows))
		return 0
	}

	// Detect the background before Bubble Tea takes over the terminal.
	renderer := preview.NewRenderer(lipgloss.HasDarkBackground())
	p := tea.NewProgram(tui.New(res, agents, windows, renderer), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "ayca:", err)
		return 1
	}
	return 0
}

func parseAgents(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []string
	for _, a := range strings.Split(s, ",") {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if !slices.Contains(discovery.AllAgents, a) {
			return nil, fmt.Errorf("unknown agent %q (supported: %s)", a, strings.Join(discovery.AllAgents, ", "))
		}
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out, nil
}

func printList(w io.Writer, res discovery.Result) {
	env := res.Env
	fmt.Fprintf(w, "Project: %s\n", env.Tilde(env.ProjectRoot))
	if env.Cwd != env.ProjectRoot {
		fmt.Fprintf(w, "Cwd:     %s\n", env.Tilde(env.Cwd))
	}
	for _, cat := range tree.Build(res.Items) {
		fmt.Fprintf(w, "\n%s\n", cat.Label)
		for _, sc := range cat.Children {
			fmt.Fprintf(w, "  %s\n", sc.Label)
			for _, n := range sc.Children {
				it := n.Item
				line := fmt.Sprintf("    %-40s [%s]  %s", it.Name, strings.Join(it.Agents, ","), env.Tilde(it.Path))
				if it.Target != "" {
					line += " -> " + env.Tilde(it.Target)
				}
				if it.Note != "" {
					line += "  (" + it.Note + ")"
				}
				fmt.Fprintln(w, line)
				for _, a := range it.Aliases {
					fmt.Fprintf(w, "      also at %s\n", env.Tilde(a))
				}
			}
		}
	}
	if len(res.Warnings) > 0 {
		fmt.Fprintf(w, "\nWarnings\n")
		for _, wr := range res.Warnings {
			fmt.Fprintf(w, "  %s: %s\n", env.Tilde(wr.Path), wr.Err)
		}
	}
}

func printBudget(w io.Writer, rep budget.Report) {
	fmt.Fprintf(w, "\nStartup context (estimate: bytes/4, files on disk only)\n")
	for _, a := range rep.Agents {
		fmt.Fprintf(w, "  %-8s ~%s tokens  %.1f%% of %s\n", a.Agent, budget.Human(a.Tokens), a.Percent(), budget.Human(a.Window))
		for _, st := range a.ByCategory() {
			fmt.Fprintf(w, "    %-14s %3d item%-2s %7s tokens\n", tree.CategoryLabel(st.Category), st.Count, map[bool]string{true: "", false: "s"}[st.Count == 1], budget.Human(st.Tokens))
		}
		for _, e := range a.Entries {
			fmt.Fprintf(w, "    %7s  %-40s %s\n", budget.Human(e.Tokens), e.Name, e.How)
		}
		for _, n := range a.Notes {
			fmt.Fprintf(w, "    note: %s\n", n)
		}
		if len(a.NotCounted) > 0 {
			fmt.Fprintf(w, "    not counted: %s\n", strings.Join(a.NotCounted, "; "))
		}
	}
}
