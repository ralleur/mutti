// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Module data in a backup: owner configuration with profile-scoped service
// credentials, conversations, chat attachments, upload task ownership, the
// content ID index and the action journal (so references and recorded
// outcomes survive a restore and an unknown outcome is never repeated).
// Engine models are re-downloadable artifacts and are deliberately excluded.
var hubBackupName = regexp.MustCompile(`^hub/(hub\.json|document-tasks\.json|content-ids\.json|actions\.json|ai/conversations/[0-9a-f]{32}/[0-9a-f]{32}\.json|ai/attachments/[0-9a-f]{32}/[0-9a-f]{32}/[0-9a-f]{32}(\.txt)?)$`)

func (m *Manager) snapshotHub(target string) ([]string, error) {
	source := m.hubDirectory()
	if _, err := os.Stat(filepath.Join(source, "hub.json")); os.IsNotExist(err) {
		return nil, nil
	}
	var names []string
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, path)
		if d.IsDir() {
			if rel == "ai/models" || rel == "ai/home" || rel == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		name := "hub/" + filepath.ToSlash(rel)
		if !hubBackupName.MatchString(name) || !d.Type().IsRegular() {
			return nil
		}
		names = append(names, name)
		return copyPrivate(path, filepath.Join(filepath.Dir(target), filepath.FromSlash(name)))
	})
	return names, err
}

func copyPrivate(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if syncErr := out.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

// restoreHub replaces module data with the verified backup copy. The replaced
// files are kept in a dated folder; downloaded models stay in place.
func (m *Manager) restoreHub(b *storedBackup) error {
	if !b.Hub {
		return nil
	}
	m.hub.stop()
	m.hub = nil
	current := m.hubDirectory()
	keep := filepath.Join(m.Options.Root, "hub-before-restore-"+time.Now().UTC().Format("20060102-150405"))
	for _, rel := range []string{"hub.json", "document-tasks.json", "content-ids.json", "actions.json", "ai/conversations", "ai/attachments"} {
		from := filepath.Join(current, filepath.FromSlash(rel))
		if _, err := os.Lstat(from); err == nil {
			to := filepath.Join(keep, filepath.FromSlash(rel))
			if err = os.MkdirAll(filepath.Dir(to), 0700); err != nil {
				return err
			}
			if err = os.Rename(from, to); err != nil {
				return err
			}
		}
	}
	for name := range b.Hashes {
		if !strings.HasPrefix(name, "hub/") {
			continue
		}
		rel := strings.TrimPrefix(name, "hub/")
		if err := copyPrivate(filepath.Join(b.directory, filepath.FromSlash(name)), filepath.Join(current, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("Die Moduldaten der Sicherung konnten nicht vollständig übernommen werden; der vorherige Stand liegt in %s.", keep)
		}
	}
	return nil
}
