package jsonout

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R037/ayca/internal/budget"
	"github.com/R037/ayca/internal/discovery"
)

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (discovery.Result, budget.Report, string) {
	t.Helper()
	root := t.TempDir()
	home, proj := filepath.Join(root, "home"), filepath.Join(root, "proj")
	for _, d := range []string{filepath.Join(proj, ".git"), home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(proj, "CLAUDE.md"), strings.Repeat("x", 401)+"\n@docs/extra.md\n")
	write(t, filepath.Join(proj, "docs", "extra.md"), strings.Repeat("y", 100))
	write(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers":{"srv":{"command":"x","env":{"API_KEY":"secret123"}}}}`)
	env := discovery.NewEnvWith(proj, home, func(string) string { return "" })
	env.ManagedDir = filepath.Join(root, "managed")
	res := discovery.Discover(env, nil)
	return res, budget.Estimate(res, nil, nil), proj
}

func find(items []Item, pred func(Item) bool) *Item {
	for i := range items {
		if pred(items[i]) {
			return &items[i]
		}
		if c := find(items[i].Children, pred); c != nil {
			return c
		}
	}
	return nil
}

func encode(t *testing.T, d Doc) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, d); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRoundTripAndPaths(t *testing.T) {
	res, rep, proj := fixture(t)
	raw := encode(t, Build(res, rep, "v1"))
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.SchemaVersion != 1 || d.AycaVersion != "v1" || d.Project.Root != proj {
		t.Fatalf("header: %+v", d)
	}
	if len(d.Items) == 0 || len(d.Budget) != len(discovery.AllAgents) {
		t.Fatalf("items=%d budget=%d", len(d.Items), len(d.Budget))
	}
	var check func([]Item)
	check = func(items []Item) {
		for _, it := range items {
			if !it.Virtual && !filepath.IsAbs(it.Path) {
				t.Errorf("path not absolute: %q", it.Path)
			}
			if strings.HasPrefix(it.Path, "~") {
				t.Errorf("tilde path: %q", it.Path)
			}
			check(it.Children)
		}
	}
	check(d.Items)
	if strings.Contains(string(raw), "secret123") {
		t.Error("secret leaked")
	}
}

func TestStartupAndSize(t *testing.T) {
	res, rep, proj := fixture(t)
	d := Build(res, rep, "v")

	claude := find(d.Items, func(i Item) bool { return i.Path == filepath.Join(proj, "CLAUDE.md") })
	if claude == nil {
		t.Fatal("CLAUDE.md missing")
	}
	if claude.SizeBytes <= 401 || claude.TokensEst != (claude.SizeBytes+3)/4 {
		t.Errorf("size/tokens: %d %d", claude.SizeBytes, claude.TokensEst)
	}
	if len(claude.Startup) == 0 || claude.Startup[0].Agent != "claude" || claude.Startup[0].Tokens == 0 {
		t.Errorf("startup: %+v", claude.Startup)
	}
	imp := find(claude.Children, func(i Item) bool { return filepath.Base(i.Path) == "extra.md" })
	if imp == nil || len(imp.Startup) == 0 || imp.TokensEst != 25 {
		t.Fatalf("import child: %+v", imp)
	}
	other := find(d.Items, func(i Item) bool { return i.Path == filepath.Join(proj, ".mcp.json") && i.Virtual })
	if other == nil {
		t.Fatal("mcp item missing")
	}
	if other.SizeBytes != 0 || other.TokensEst != 0 {
		t.Errorf("virtual item has size: %+v", other)
	}
	// Every budget entry refers to an item that carries the startup entry.
	for _, a := range d.Budget {
		for _, e := range a.Entries {
			it := find(d.Items, func(i Item) bool { return i.ID == e.ItemID })
			if it == nil {
				t.Errorf("entry %s has no item", e.ItemID)
			}
		}
	}
}

func TestDirNoSize(t *testing.T) {
	it := discovery.Item{ID: "d", Path: t.TempDir(), IsDir: true}
	out := buildItem(&it, nil)
	if out.SizeBytes != 0 || out.TokensEst != 0 {
		t.Errorf("dir sized: %+v", out)
	}
	miss := discovery.Item{ID: "m", Path: "/nonexistent/x", Missing: true}
	if o := buildItem(&miss, nil); o.SizeBytes != 0 || !o.Missing {
		t.Errorf("missing: %+v", o)
	}
}

func TestEmptyArraysAndDeterminism(t *testing.T) {
	raw := encode(t, Build(discovery.Result{}, budget.Report{}, "v"))
	for _, k := range []string{`"items": []`, `"budget": []`, `"warnings": []`} {
		if !bytes.Contains(raw, []byte(k)) {
			t.Errorf("missing %s in %s", k, raw)
		}
	}
	res, rep, _ := fixture(t)
	a, b := encode(t, Build(res, rep, "v")), encode(t, Build(res, rep, "v"))
	if !bytes.Equal(a, b) {
		t.Error("not deterministic")
	}
	if bytes.Contains(a, []byte(`<`)) {
		t.Error("HTML escaped")
	}
}
