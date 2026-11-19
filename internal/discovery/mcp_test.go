package discovery

import (
	"reflect"
	"testing"
)

func TestMaskSecrets(t *testing.T) {
	in := map[string]any{
		"command": "npx",
		"args": []any{
			"-y", "server",
			"--api-key", "plain-secret",
			"--token=abc123",
			"--token", "${GH_TOKEN}",
			"-e", "API_KEY=xyz",
			"--port", "8080",
			"sk-abcdefghijklmnop",
		},
		"env": map[string]any{
			"FOO_ENV":   "value",
			"FROM_VAR":  "${SECRET}",
			"PLAIN_VAL": "abc",
		},
		"headers":              map[string]any{"X-Custom": "anything"},
		"url":                  "https://user:pass@example.com/mcp?key=1&page=2",
		"bearer_token_env_var": "MY_TOKEN",
		"client_secret":        "s3cr3t",
		"timeout":              30.0,
		"enabled":              true,
		"description":          "Bearer abcdef",
	}
	got := MaskSecrets(in).(map[string]any)
	want := map[string]any{
		"command": "npx",
		"args": []any{
			"-y", "server",
			"--api-key", Masked,
			"--token=" + Masked,
			"--token", "${GH_TOKEN}",
			"-e", "API_KEY=" + Masked,
			"--port", "8080",
			Masked,
		},
		"env": map[string]any{
			"FOO_ENV":   Masked,
			"FROM_VAR":  "${SECRET}",
			"PLAIN_VAL": Masked,
		},
		"headers":              map[string]any{"X-Custom": Masked},
		"url":                  "https://user:" + Masked + "@example.com/mcp?key=" + Masked + "&page=2",
		"bearer_token_env_var": "MY_TOKEN",
		"client_secret":        Masked,
		"timeout":              30.0,
		"enabled":              true,
		"description":          Masked,
	}
	for k := range want {
		if !reflect.DeepEqual(got[k], want[k]) {
			t.Errorf("%s:\n got  %#v\n want %#v", k, got[k], want[k])
		}
	}
	// Input must be untouched.
	if in["env"].(map[string]any)["PLAIN_VAL"] != "abc" {
		t.Error("MaskSecrets mutated its input")
	}
}
