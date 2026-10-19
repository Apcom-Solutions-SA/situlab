// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 APCOM Solutions SA

package core

import "testing"

func TestParseDefaultGateway(t *testing.T) {
	cases := map[string]string{
		"default via 192.168.32.1 dev eth0 proto kernel \n192.168.32.0/20 dev eth0 proto kernel scope link": "192.168.32.1",
		"default via 172.20.80.1 dev eth0":                     "172.20.80.1",
		"192.168.42.0/20 dev eth0 proto kernel scope link src": "", // pas de route par défaut
		"":                 "",
		"default dev eth0": "", // « default » sans « via »
	}
	for in, want := range cases {
		if got := parseDefaultGateway(in); got != want {
			t.Errorf("parseDefaultGateway(%q) = %q, want %q", in, got, want)
		}
	}
}
