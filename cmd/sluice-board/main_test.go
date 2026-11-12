// SPDX-License-Identifier: AGPL-3.0-only

package main

import "testing"

func TestValidRepo(t *testing.T) {
	for r, want := range map[string]bool{
		"owner/name":   true,
		"o-1/n_2.go":   true,
		"owner/..":     false,
		"./name":       false,
		"owner":        false,
		"owner/a/b":    false,
		"owner/na me":  false,
		"owner/name?x": false,
	} {
		if got := validRepo(r); got != want {
			t.Errorf("validRepo(%q) = %v, want %v", r, got, want)
		}
	}
}

func TestLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8080": true,
		"localhost:8080": true,
		"[::1]:8080":     true,
		":8080":          false,
		"0.0.0.0:8080":   false,
		"10.0.0.5:8080":  false,
	} {
		if got := loopback(addr); got != want {
			t.Errorf("loopback(%q) = %v, want %v", addr, got, want)
		}
	}
}
