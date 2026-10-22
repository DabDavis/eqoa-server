package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SetInFile changes settings in a config file in place, by dotted key
// ("world.start_level"), and leaves the rest of it as it was: its comments,
// its order and every other setting (the GM panel's settings page writes
// through it, so a restart keeps what was changed live). A key the file does
// not have yet is added at the end of its section. The old file is copied to
// a backup first, whose name it returns ("" when there was no file); the new
// one must still load, or nothing is written.
func SetInFile(path string, changes map[string]any) (string, error) {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	var doc yaml.Node
	if len(bytes.TrimSpace(old)) > 0 {
		if err := yaml.Unmarshal(old, &doc); err != nil {
			return "", fmt.Errorf("%s does not parse: %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("%s is not a mapping of settings", path)
	}

	for key, value := range changes {
		if err := setNode(doc.Content[0], strings.Split(key, "."), value); err != nil {
			return "", fmt.Errorf("%s: %w", key, err)
		}
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", err
	}
	enc.Close()

	// what is written must load as a Config
	check := DefaultConfig()
	if err := yaml.Unmarshal(out.Bytes(), check); err != nil {
		return "", fmt.Errorf("the changed file would not load: %w", err)
	}

	mode := os.FileMode(0644)
	backup := ""
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
		backup = fmt.Sprintf("%s.bak-%s-gm", path, time.Now().UTC().Format("20060102-150405"))
		if err := os.WriteFile(backup, old, mode); err != nil {
			return "", fmt.Errorf("backup: %w", err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return backup, err
	}
	if _, err := tmp.Write(out.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return backup, err
	}
	tmp.Close()
	os.Chmod(tmp.Name(), mode)
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return backup, err
	}
	return backup, nil
}

// setNode sets the value at path under a mapping node, making the mappings on
// the way when they are missing. A replaced value keeps the comments it had.
func setNode(m *yaml.Node, path []string, value any) error {
	if m.Kind != yaml.MappingNode {
		return fmt.Errorf("%q is not a section", path[0])
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != path[0] {
			continue
		}
		if len(path) > 1 {
			if v.Kind == yaml.ScalarNode && (v.Tag == "!!null" || v.Value == "") {
				v.Kind, v.Tag, v.Value, v.Content = yaml.MappingNode, "!!map", "", nil
			}
			return setNode(v, path[1:], value)
		}
		var nv yaml.Node
		if err := nv.Encode(value); err != nil {
			return err
		}
		nv.HeadComment, nv.LineComment, nv.FootComment = v.HeadComment, v.LineComment, v.FootComment
		if nv.Kind == yaml.MappingNode || nv.Kind == yaml.SequenceNode {
			nv.Style = 0 // block style, as the rest of the file is written
		}
		*v = nv
		return nil
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[0]}
	if len(path) > 1 {
		sub := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		m.Content = append(m.Content, key, sub)
		return setNode(sub, path[1:], value)
	}
	var nv yaml.Node
	if err := nv.Encode(value); err != nil {
		return err
	}
	m.Content = append(m.Content, key, &nv)
	return nil
}
