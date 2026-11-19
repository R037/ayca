package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONListConflict(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--json", "--list"}, &out, &errb); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if out.Len() != 0 || !strings.Contains(errb.String(), "--json") {
		t.Errorf("out=%q err=%q", out.String(), errb.String())
	}
}

func TestJSONOutput(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--json", "--path", t.TempDir(), "--agent", "claude", "--window", "100k"}, &out, &errb); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	var d struct {
		SchemaVersion int `json:"schema_version"`
		Budget        []struct {
			Agent  string `json:"agent"`
			Window int    `json:"window"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.SchemaVersion != 1 || len(d.Budget) != 1 || d.Budget[0].Agent != "claude" || d.Budget[0].Window != 100000 {
		t.Errorf("%+v", d)
	}
}
