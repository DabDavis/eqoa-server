package config

import (
	"bytes"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The example config is the hosting template (README, "Configure"): it must
// parse as a Config with no key the server does not know, and name every key
// the server reads, set or commented out, so it cannot drift from config.go.

const examplePath = "../../config.yaml.example"

// yamlTags is every yaml key of t and the structs below it (slices, maps and
// pointers followed; an inline struct's keys counted as its parent's).
func yamlTags(t reflect.Type, into map[string]bool, seen map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if name != "" {
			into[name] = true
		} else if !strings.Contains(opts, "inline") && f.IsExported() {
			into[strings.ToLower(f.Name)] = true
		}
		yamlTags(f.Type, into, seen)
	}
}

func configTags() map[string]bool {
	tags := map[string]bool{}
	yamlTags(reflect.TypeOf(Config{}), tags, map[reflect.Type]bool{})
	return tags
}

func readExample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The file's live (uncommented) settings decode into Config with no unknown key.
func TestExampleParsesStrictly(t *testing.T) {
	dec := yaml.NewDecoder(bytes.NewReader(readExample(t)))
	dec.KnownFields(true)
	cfg := DefaultConfig()
	if err := dec.Decode(cfg); err != nil {
		t.Fatalf("config.yaml.example: %v", err)
	}
	if len(cfg.Auth.Accounts) != 0 {
		t.Fatal("the example must leave account credentials to the operator")
	}
	// And through Load, as the server reads it.
	if _, err := Load(examplePath); err != nil {
		t.Fatal(err)
	}
}

// A commented-out setting ("# key: value", "#   key: value", "# - key: value")
// is one a reader uncomments, so its key must be one the server knows too.
var commentedKey = regexp.MustCompile(`^\s*#\s+(?:-\s+)?([a-z][a-z0-9_]*):(?:\s|$)`)

// Keys written in a flow mapping ({weapon: 80, shield: 0}).
var flowKey = regexp.MustCompile(`[{,]\s*([a-z][a-z0-9_]*):`)

func TestExampleCommentedKeysAreKnown(t *testing.T) {
	tags := configTags()
	var unknown []string
	for n, line := range strings.Split(string(readExample(t)), "\n") {
		m := commentedKey.FindStringSubmatch(line)
		if m == nil || tags[m[1]] {
			continue
		}
		unknown = append(unknown, m[1]+" (line "+itoa(n+1)+")")
	}
	if len(unknown) > 0 {
		t.Errorf("config.yaml.example comments out keys Config does not have: %s", strings.Join(unknown, ", "))
	}
}

// Every key Config reads is named in the example, set or commented out.
func TestExampleNamesEveryKey(t *testing.T) {
	named := map[string]bool{}
	setting := regexp.MustCompile(`^[\s#-]*([a-z][a-z0-9_]*):(?:\s|$)`)
	for _, line := range strings.Split(string(readExample(t)), "\n") {
		if m := setting.FindStringSubmatch(line); m != nil {
			named[m[1]] = true
		}
		for _, m := range flowKey.FindAllStringSubmatch(line, -1) {
			named[m[1]] = true
		}
	}
	var missing []string
	for tag := range configTags() {
		if !named[tag] {
			missing = append(missing, tag)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("config.yaml.example does not name %d key(s) of Config; add each, commented out with its default if a fresh server does not need it: %s",
			len(missing), strings.Join(missing, ", "))
	}
}

func itoa(n int) string {
	var b [20]byte
	i := len(b)
	if n == 0 {
		return "0"
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
