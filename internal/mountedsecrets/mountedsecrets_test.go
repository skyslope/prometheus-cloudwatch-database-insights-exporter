package mountedsecrets

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// Inject errors instead of chmod: production and CI may run as root.
type failingFS struct {
	fs.FS
	failed string
}

func (f failingFS) Open(name string) (fs.File, error) {
	if name == f.failed {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}
func (f failingFS) Stat(name string) (fs.FileInfo, error) { return fs.Stat(f.FS, name) }

type failedChildStatFS struct{ fs.FS }

func (f failedChildStatFS) Stat(name string) (fs.FileInfo, error) {
	if name == "db/MSTEST_KEY" {
		return nil, fs.ErrPermission
	}
	return fs.Stat(f.FS, name)
}

type failedStatFS struct{ fs.FS }

func (f failedStatFS) Stat(name string) (fs.FileInfo, error) {
	if name == "db" {
		return nil, fs.ErrPermission
	}
	return fs.Stat(f.FS, name)
}

func TestContract(t *testing.T) {
	data := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	for _, tc := range []struct {
		name      string
		files     fs.FS
		env, want map[string]string
		warning   string
	}{
		{"invalid dynamic blocks static", fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "db/MSTEST_KEY/nested": data("deep-secret")}, nil, map[string]string{}, "not a regular file"},
		{"invalid dynamic preserves empty env", fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "db/MSTEST_KEY/nested": data("deep-secret")}, map[string]string{"MSTEST_KEY": ""}, map[string]string{"MSTEST_KEY": ""}, ""},
		{"dynamic stat failure preserves env", failedChildStatFS{fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "db/MSTEST_KEY": data("file-secret")}}, map[string]string{"MSTEST_KEY": "env-secret"}, map[string]string{"MSTEST_KEY": "env-secret"}, ""},
		{"dynamic stat failure blocks only its key", failedChildStatFS{fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "db/MSTEST_KEY": data("file-secret"), "OTHER": data("ok")}}, nil, map[string]string{"OTHER": "ok"}, "MSTEST_KEY left unset"},
		{"static", fstest.MapFS{"MSTEST_KEY": data(" value\n")}, nil, map[string]string{"MSTEST_KEY": "value"}, ""},
		{"dynamic first", fstest.MapFS{"MSTEST_KEY": data("stale"), "db/MSTEST_KEY": data("dynamic")}, nil, map[string]string{"MSTEST_KEY": "dynamic"}, ""},
		{"env wins", fstest.MapFS{"MSTEST_KEY": data("file-secret")}, map[string]string{"MSTEST_KEY": "env-secret"}, map[string]string{"MSTEST_KEY": "env-secret"}, ""},
		{"empty env wins", fstest.MapFS{"MSTEST_KEY": data("file-secret")}, map[string]string{"MSTEST_KEY": ""}, map[string]string{"MSTEST_KEY": ""}, ""},
		{"sorted directories", fstest.MapFS{"z/MSTEST_KEY": data("last"), "a/MSTEST_KEY": data("first")}, nil, map[string]string{"MSTEST_KEY": "first"}, ""},
		{"skip hidden and deeper", fstest.MapFS{".hidden": data("hidden-secret"), "db/.hidden": data("hidden-secret"), "db/deeper/MSTEST_KEY": data("deep-secret")}, nil, map[string]string{}, "not a regular file"},
		{"unreadable dynamic", failingFS{fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "db/MSTEST_KEY": data("file-secret"), "OTHER": data("ok")}, "db/MSTEST_KEY"}, nil, map[string]string{"OTHER": "ok"}, "MSTEST_KEY left unset"},
		{"uninspectable root entry", failedStatFS{fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "db/MSTEST_KEY": data("file-secret")}}, nil, map[string]string{}, "cannot inspect"},
		{"unreadable root", failingFS{fstest.MapFS{"MSTEST_KEY": data("file-secret")}, "."}, nil, map[string]string{}, "cannot list"},
		{"failed scan blocks all fallback", failingFS{fstest.MapFS{"MSTEST_KEY": data("stale-secret"), "a/MSTEST_KEY": data("file-secret"), "z/MSTEST_KEY": data("file-secret")}, "z"}, map[string]string{"EXISTING": "env-secret"}, map[string]string{"EXISTING": "env-secret"}, "cannot list"},
		{"unreadable subdir", failingFS{fstest.MapFS{"db/MSTEST_KEY": data("file-secret"), "OTHER": data("ok")}, "db"}, nil, map[string]string{}, "cannot list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range tc.env {
				env[k] = v
			}
			var logs []string
			load(tc.files, "/opt/secrets", func(k string) (string, bool) { v, ok := env[k]; return v, ok }, func(k, v string) error { env[k] = v; return nil }, func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
			if !reflect.DeepEqual(env, tc.want) {
				t.Fatalf("unexpected keys/values in resulting environment")
			}
			joined := strings.Join(logs, "\n")
			if tc.warning != "" && !strings.Contains(joined, tc.warning) {
				t.Fatalf("missing warning: %s", tc.warning)
			}
			if tc.warning == "" && len(logs) != 0 {
				t.Fatalf("unexpected warnings: %s", joined)
			}
			for _, secret := range []string{"file-secret", "env-secret", "stale-secret", "hidden-secret", "deep-secret"} {
				if strings.Contains(joined, secret) {
					t.Fatal("secret value leaked in warning")
				}
			}
		})
	}
}

func TestMissingAndNonDirectoryRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(root, "missing"), file, "", "  "} {
		before := os.Environ()
		Load(p)
		if !reflect.DeepEqual(before, os.Environ()) {
			t.Fatal("invalid root changed environment")
		}
	}
}

func TestProjectedVolumeSymlinks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MSTEST_PROJECTED", "original")
	if err := os.Unsetenv("MSTEST_PROJECTED"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "..version", "db"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "..version", "db", "MSTEST_PROJECTED"), []byte("projected\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..version", filepath.Join(root, "..data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..data/db", filepath.Join(root, "db")); err != nil {
		t.Fatal(err)
	}
	Load(root)
	if os.Getenv("MSTEST_PROJECTED") != "projected" {
		t.Fatal("projected directory symlink not followed")
	}
}

func TestSetFailureDoesNotExposeValue(t *testing.T) {
	var logs []string
	load(fstest.MapFS{"MSTEST_KEY": {Data: []byte("file-secret")}}, "/opt/secrets", func(string) (string, bool) { return "", false }, func(string, string) error { return fmt.Errorf("file-secret") }, func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
	if len(logs) != 1 || !strings.Contains(logs[0], "cannot set MSTEST_KEY") || strings.Contains(logs[0], "file-secret") {
		t.Fatal("unsafe setenv error handling")
	}
}
