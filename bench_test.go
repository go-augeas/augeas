// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import "testing"

// Representative real-world configuration fragments for the hot-path
// benchmarks. See BENCHMARKS.md for the methodology used to compare against the
// reference C Augeas (augtool/libaugeas).
var (
	benchHosts = "127.0.0.1 localhost localhost.localdomain\n" +
		"192.168.0.1 host.example.com host\n" +
		"# a comment\n" +
		"10.0.0.2 other.example.com other alias2\n"

	benchFstab = "# /etc/fstab\n" +
		"UUID=1234-5678 / ext4 defaults 0 1\n" +
		"/dev/sda2 /home ext4 defaults,noatime 0 2\n" +
		"tmpfs /tmp tmpfs defaults 0 0\n"

	benchSshd = "Port 22\n" +
		"Protocol 2\n" +
		"PermitRootLogin no\n" +
		"PasswordAuthentication yes\n" +
		"# comment\n" +
		"AllowUsers alice bob\n"
)

func benchLens(b *testing.B, module, binding string) Lens {
	b.Helper()
	e := NewEngine()
	l, err := e.Lens(module, binding)
	if err != nil {
		b.Fatalf("lens %s.%s: %v", module, binding, err)
	}
	return l
}

// BenchmarkCompile measures compiling (interpreting) a lens from source.
func BenchmarkCompileHosts(b *testing.B) {
	for i := 0; i < b.N; i++ {
		e := NewEngine()
		if _, err := e.Lens("Hosts", "lns"); err != nil {
			b.Fatal(err)
		}
	}
}

func benchGet(b *testing.B, module, binding, text string) {
	l := benchLens(b, module, binding)
	b.ResetTimer()
	b.SetBytes(int64(len(text)))
	for i := 0; i < b.N; i++ {
		if _, err := l.Parse(text); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetHosts(b *testing.B) { benchGet(b, "Hosts", "lns", benchHosts) }
func BenchmarkGetFstab(b *testing.B) { benchGet(b, "Fstab", "lns", benchFstab) }
func BenchmarkGetSshd(b *testing.B)  { benchGet(b, "Sshd", "lns", benchSshd) }
