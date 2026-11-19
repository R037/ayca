// Package jsonout converts discovery and budget results into a stable,
// machine-readable JSON document. It uses dedicated output structs so that
// internal renames cannot change the format. It is read-only.
package jsonout

import (
	"encoding/json"
	"io"
	"os"

	"github.com/R037/ayca/internal/budget"
	"github.com/R037/ayca/internal/discovery"
)

// SchemaVersion is bumped on incompatible changes to the document.
const SchemaVersion = 1

// Doc is the top-level JSON document.
type Doc struct {
	SchemaVersion int       `json:"schema_version"`
	AycaVersion   string    `json:"ayca_version"`
	Project       Project   `json:"project"`
	OutputStyle   string    `json:"output_style,omitempty"`
	Items         []Item    `json:"items"`
	Budget        []Agent   `json:"budget"`
	Warnings      []Warning `json:"warnings"`
}

// Project locates the inspected project.
type Project struct {
	Root string `json:"root"`
	Cwd  string `json:"cwd"`
}

// Item is one discovered piece of context.
type Item struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Target      string         `json:"target,omitempty"`
	Aliases     []string       `json:"aliases,omitempty"`
	Category    string         `json:"category"`
	Scope       string         `json:"scope"`
	Agents      []string       `json:"agents"`
	IsDir       bool           `json:"is_dir,omitempty"`
	Virtual     bool           `json:"virtual,omitempty"`
	Missing     bool           `json:"missing,omitempty"`
	DeferredFor []string       `json:"deferred_for,omitempty"`
	Note        string         `json:"note,omitempty"`
	Info        string         `json:"info,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	SizeBytes   int64          `json:"size_bytes,omitempty"`
	TokensEst   int64          `json:"tokens_est,omitempty"`
	Startup     []Startup      `json:"startup,omitempty"`
	Children    []Item         `json:"children,omitempty"`
}

// Startup says how an item counts toward one agent's startup context.
type Startup struct {
	Agent  string `json:"agent"`
	Tokens int    `json:"tokens"`
	How    string `json:"how"`
}

// Agent is the startup budget of one agent.
type Agent struct {
	Agent      string     `json:"agent"`
	Window     int        `json:"window"`
	Tokens     int        `json:"tokens"`
	Percent    float64    `json:"percent"`
	ByCategory []Category `json:"by_category"`
	Entries    []Entry    `json:"entries"`
	NotCounted []string   `json:"not_counted,omitempty"`
	Notes      []string   `json:"notes,omitempty"`
}

// Category is a per-category subtotal.
type Category struct {
	Category string `json:"category"`
	Items    int    `json:"items"`
	Tokens   int    `json:"tokens"`
}

// Entry is one counted startup entry.
type Entry struct {
	ItemID   string `json:"item_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Tokens   int    `json:"tokens"`
	How      string `json:"how"`
}

// Warning is a non-fatal discovery problem.
type Warning struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Build converts a discovery result and budget report into a Doc.
func Build(res discovery.Result, rep budget.Report, version string) Doc {
	d := Doc{
		SchemaVersion: SchemaVersion,
		AycaVersion:   version,
		Project:       Project{Root: res.Env.ProjectRoot, Cwd: res.Env.Cwd},
		OutputStyle:   res.OutputStyle,
		Items:         []Item{},
		Budget:        []Agent{},
		Warnings:      []Warning{},
	}

	// Index startup entries by item ID, per agent, in report order.
	startup := map[string][]Startup{}
	for _, a := range rep.Agents {
		sums := map[string]int{}
		var order []string
		hows := map[string]string{}
		for _, e := range a.Entries {
			if _, ok := sums[e.ItemID]; !ok {
				order = append(order, e.ItemID)
				hows[e.ItemID] = e.How
			}
			sums[e.ItemID] += e.Tokens
		}
		for _, id := range order {
			startup[id] = append(startup[id], Startup{Agent: a.Agent, Tokens: sums[id], How: hows[id]})
		}
	}

	for i := range res.Items {
		d.Items = append(d.Items, buildItem(&res.Items[i], startup))
	}
	for _, a := range rep.Agents {
		ag := Agent{
			Agent: a.Agent, Window: a.Window, Tokens: a.Tokens,
			Percent:    float64(int(a.Percent()*10+0.5)) / 10,
			ByCategory: []Category{}, Entries: []Entry{},
			NotCounted: a.NotCounted, Notes: a.Notes,
		}
		for _, st := range a.ByCategory() {
			ag.ByCategory = append(ag.ByCategory, Category{Category: string(st.Category), Items: st.Count, Tokens: st.Tokens})
		}
		for _, e := range a.Entries {
			ag.Entries = append(ag.Entries, Entry{ItemID: e.ItemID, Name: e.Name, Path: e.Path, Category: string(e.Category), Tokens: e.Tokens, How: e.How})
		}
		d.Budget = append(d.Budget, ag)
	}
	for _, w := range res.Warnings {
		d.Warnings = append(d.Warnings, Warning{Path: w.Path, Error: w.Err})
	}
	return d
}

func buildItem(it *discovery.Item, startup map[string][]Startup) Item {
	out := Item{
		ID: it.ID, Name: it.Name, Path: it.Path, Target: it.Target,
		Aliases: it.Aliases, Category: string(it.Category), Scope: string(it.Scope),
		Agents: it.Agents, IsDir: it.IsDir, Virtual: it.Virtual, Missing: it.Missing,
		DeferredFor: it.DeferredFor, Note: it.Note, Info: it.Info, Config: it.Config,
		Startup: startup[it.ID],
	}
	if out.Agents == nil {
		out.Agents = []string{}
	}
	if !it.IsDir && !it.Virtual && !it.Missing {
		if fi, err := os.Stat(it.Path); err == nil && fi.Mode().IsRegular() {
			out.SizeBytes = fi.Size()
			out.TokensEst = (fi.Size() + 3) / 4 // same bytes/4 rule as the preview
		}
	}
	for i := range it.Children {
		out.Children = append(out.Children, buildItem(&it.Children[i], startup))
	}
	return out
}

// Write encodes d as indented JSON without HTML escaping.
func Write(w io.Writer, d Doc) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(d)
}
