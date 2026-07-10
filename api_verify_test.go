package augeas

import (
	"io/fs"
	"strings"
	"testing"
)

type memFS struct{ files map[string]string }

func (m memFS) ReadFile(n string) ([]byte, error) {
	if s, ok := m.files[n]; ok {
		return []byte(s), nil
	}
	return nil, fs.ErrNotExist
}
func (m memFS) WriteFile(n string, d []byte, p fs.FileMode) error { m.files[n] = string(d); return nil }
func (m memFS) Glob(p string) ([]string, error) {
	var out []string
	for k := range m.files {
		if ok, _ := matchGlob(p, k); ok {
			out = append(out, k)
		}
	}
	return out, nil
}
func matchGlob(p, k string) (bool, error) {
	return strings.HasSuffix(k, strings.TrimPrefix(p, "*")), nil
}
func TestPublicHostsGet(t *testing.T) {
	a := New()
	a.SetFileSystem(memFS{files: map[string]string{"/etc/hosts": "127.0.0.1 localhost\n192.168.0.1 host.example host\n"}})
	if err := a.LoadFile("/etc/hosts"); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	v, ok := a.Get("/files/etc/hosts/1/ipaddr")
	if !ok || v != "127.0.0.1" {
		t.Fatalf("ipaddr got %q ok=%v", v, ok)
	}
	v2, _ := a.Get("/files/etc/hosts/2/canonical")
	if v2 != "host.example" {
		t.Fatalf("canonical got %q", v2)
	}
	al, _ := a.Get("/files/etc/hosts/2/alias")
	if al != "host" {
		t.Fatalf("alias got %q", al)
	}
	t.Logf("OK: ipaddr=%s canonical=%s alias=%s", v, v2, al)
}
