package discovery

import (
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Masked replaces any value AYCA considers secret.
const Masked = "••••••••"

// mcpServers adds one virtual item per server in servers.
func (c *collector) mcpServers(cfgPath string, servers map[string]any, scope Scope, note string, agents ...string) {
	for _, name := range sortedKeys(servers) {
		cfg, _ := servers[name].(map[string]any)
		if cfg == nil {
			cfg = map[string]any{}
		}
		c.add(Item{
			Name:     name,
			Path:     cfgPath,
			Category: CategoryMCP,
			Scope:    scope,
			Agents:   agents,
			Virtual:  true,
			Note:     note,
			Config:   MaskSecrets(cfg).(map[string]any),
		})
	}
}

// mcpJSONFile adds the servers found under "mcpServers" in a JSON file.
func (c *collector) mcpJSONFile(path string, scope Scope, note string, agents ...string) {
	doc, ok := c.readJSON(path)
	if !ok {
		return
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	c.mcpServers(path, servers, scope, note, agents...)
}

// mcpTOMLFile adds the servers under [mcp_servers.*] in a Codex config.toml.
func (c *collector) mcpTOMLFile(path string, scope Scope, note string, agents ...string) {
	if fi, ok := c.stat(path); !ok || !fi.Mode().IsRegular() {
		return
	}
	var doc map[string]any
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		c.warn(path, err)
		return
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	c.mcpServers(path, servers, scope, note, agents...)
}

func (c *collector) readJSON(path string) (map[string]any, bool) {
	fi, ok := c.stat(path)
	if !ok || !fi.Mode().IsRegular() {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.warn(path, err)
		return nil, false
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		c.warn(path, err)
		return nil, false
	}
	return doc, true
}

var (
	sensitiveKey = regexp.MustCompile(`(?i)(token|secret|passw(or)?d|api[-_]?key|apikey|auth|credential|private[-_]?key|cookie|session)`)
	secretValue  = regexp.MustCompile(`(?i)^(bearer\s+\S+|basic\s+\S+|sk-[a-z0-9_-]{8,}|sk_(live|test)_\S+|gh[pousr]_[a-z0-9]{16,}|github_pat_\S+|xox[abpors]-\S+|AKIA[0-9A-Z]{12,}|AIza[0-9A-Za-z_-]{20,}|glpat-\S+|eyJ[a-z0-9_-]{10,}\.[a-z0-9_-]+\.\S*)$`)
	envRef       = regexp.MustCompile(`^\$\{?(env:)?[A-Za-z_][A-Za-z0-9_]*\}?$`)
	// Query parameters often use short names for credentials (?key=, ?sig=).
	sensitiveQuery = regexp.MustCompile(`(?i)^(key|sig|signature|code|pass|pw)$`)
	flagWithVal    = regexp.MustCompile(`^(--?[A-Za-z0-9_-]+)=(.*)$`)
)

// MaskSecrets returns a deep copy of v with likely secrets replaced by
// Masked. It errs on the side of masking: values of env/headers maps,
// values of keys that look sensitive, arguments following sensitive flags,
// URL credentials and query parameters, and well-known token formats.
// References to environment variables ($VAR, ${VAR}) are kept, since they
// are not secrets themselves and are useful to see.
func MaskSecrets(v any) any {
	return mask(v, "", false)
}

func mask(v any, key string, all bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		maskChildren := strings.EqualFold(key, "env") || strings.EqualFold(key, "headers") ||
			strings.EqualFold(key, "environment") || strings.EqualFold(key, "http_headers")
		for k, val := range t {
			out[k] = mask(val, k, all || maskChildren)
		}
		return out
	case []any:
		out := make([]any, len(t))
		if strings.EqualFold(key, "args") {
			for i, a := range t {
				out[i] = maskArg(t, i, a)
			}
			return out
		}
		for i, a := range t {
			out[i] = mask(a, key, all)
		}
		return out
	case string:
		return maskString(t, key, all)
	default:
		return v
	}
}

func maskString(s, key string, all bool) string {
	if s == "" || envRef.MatchString(s) {
		return s
	}
	if all {
		return Masked
	}
	// Keys that name an environment variable (e.g. bearer_token_env_var)
	// hold a variable name, not a secret.
	lk := strings.ToLower(key)
	if strings.HasSuffix(lk, "_env_var") || strings.HasSuffix(lk, "env_vars") {
		return s
	}
	if sensitiveKey.MatchString(key) || looksSecret(s) {
		return Masked
	}
	if strings.Contains(s, "://") {
		return maskURL(s)
	}
	return s
}

func maskArg(args []any, i int, a any) any {
	s, ok := a.(string)
	if !ok {
		return mask(a, "", false)
	}
	if m := flagWithVal.FindStringSubmatch(s); m != nil && sensitiveKey.MatchString(m[1]) && !envRef.MatchString(m[2]) {
		return m[1] + "=" + Masked
	}
	if i > 0 {
		if prev, ok := args[i-1].(string); ok && strings.HasPrefix(prev, "-") && !strings.Contains(prev, "=") &&
			sensitiveKey.MatchString(prev) && !envRef.MatchString(s) {
			return Masked
		}
	}
	// KEY=value style arguments (e.g. docker -e API_KEY=...).
	if k, v, ok := strings.Cut(s, "="); ok && !strings.HasPrefix(k, "-") && sensitiveKey.MatchString(k) && !envRef.MatchString(v) {
		return k + "=" + Masked
	}
	return maskString(s, "", false)
}

func looksSecret(s string) bool {
	return secretValue.MatchString(strings.TrimSpace(s))
}

func maskURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	changed := false
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
			changed = true
		}
	}
	q := u.Query()
	for k, vals := range q {
		for i, v := range vals {
			if sensitiveKey.MatchString(k) || sensitiveQuery.MatchString(k) || looksSecret(v) {
				vals[i] = "xxxxx"
				changed = true
			}
		}
		q[k] = vals
	}
	if !changed {
		return s
	}
	u.RawQuery = q.Encode()
	return strings.ReplaceAll(u.String(), "xxxxx", Masked)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}
