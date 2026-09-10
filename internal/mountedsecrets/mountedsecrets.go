// Package mountedsecrets loads projected secret files before application config.
package mountedsecrets

import (
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Load preserves existing environment variables, including empty values.
// Dynamic files one level below root take precedence over top-level KV files.
// Missing mounts are a no-op; failures warn without logging secret contents.
func Load(root string) {
	load(os.DirFS(root), root, os.LookupEnv, os.Setenv, log.Printf)
}

func load(files fs.FS, root string, lookup func(string) (string, bool), set func(string, string) error, warn func(string, ...any)) {
	info, err := fs.Stat(files, ".")
	if err != nil {
		if !os.IsNotExist(err) {
			warn("[mounted-secrets] cannot inspect %s", root)
		}
		return
	}
	if !info.IsDir() {
		warn("[mounted-secrets] not a directory: %s", root)
		return
	}
	list := func(dir string) []fs.DirEntry {
		entries, err := fs.ReadDir(files, dir)
		if err != nil {
			warn("[mounted-secrets] cannot list %s", filepath.Join(root, dir))
			return nil
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		return entries
	}
	claimed := make(map[string]bool)
	read := func(name, key string) {
		if claimed[key] {
			return
		}
		claimed[key] = true // Claim before reading: never fall back to stale KV.
		if _, exists := lookup(key); exists {
			warn("[mounted-secrets] %s already set; ignoring %s", key, filepath.Join(root, name))
			return
		}
		value, err := fs.ReadFile(files, name)
		if err != nil {
			warn("[mounted-secrets] cannot read %s; %s left unset", filepath.Join(root, name), key)
			return
		}
		if err := set(key, strings.TrimSpace(string(value))); err != nil {
			warn("[mounted-secrets] cannot set %s from %s", key, filepath.Join(root, name))
		}
	}
	entries := list(".")
	var static []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Stat follows Kubernetes projected-volume symlinks, unlike IsDir.
		info, err := fs.Stat(files, name)
		if err != nil {
			warn("[mounted-secrets] cannot inspect %s", filepath.Join(root, name))
			continue
		}
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				static = append(static, name)
			}
			continue
		}
		for _, child := range list(name) {
			key := child.Name()
			if strings.HasPrefix(key, ".") {
				continue
			}
			file := path.Join(name, key)
			info, err := fs.Stat(files, file)
			if err != nil {
				claimed[key] = true
				warn("[mounted-secrets] cannot inspect %s; %s left unset", filepath.Join(root, file), key)
				continue
			}
			if info.Mode().IsRegular() {
				read(file, key)
			}
		}
	}
	for _, name := range static {
		read(name, name)
	}
}
