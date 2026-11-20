package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvWithCwd(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	os.MkdirAll(filepath.Join(proj, ".git"), 0o755)
	os.MkdirAll(filepath.Join(proj, "sub"), 0o755)
	other := filepath.Join(root, "other", "repo")
	os.MkdirAll(filepath.Join(other, ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "plain"), 0o755)

	env := NewEnvWith(proj, root, func(string) string { return "" })
	env.ManagedDir = filepath.Join(root, "managed")

	sub := env.WithCwd(proj + "/sub/")
	if sub.Cwd != filepath.Join(proj, "sub") || sub.ProjectRoot != proj {
		t.Errorf("inside root: %+v", sub)
	}
	if sub.ManagedDir != env.ManagedDir || sub.Home != env.Home {
		t.Error("other fields must be copied")
	}
	if same := env.WithCwd(proj); same.Cwd != proj || same.ProjectRoot != proj {
		t.Errorf("same dir: %+v", same)
	}
	if o := env.WithCwd(other); o.ProjectRoot != other {
		t.Errorf("other repo: %+v", o)
	}
	if p := env.WithCwd(filepath.Join(root, "plain")); p.ProjectRoot != filepath.Join(root, "plain") {
		t.Errorf("no repo: %+v", p)
	}
	if env.Cwd != proj {
		t.Error("original modified")
	}
}
