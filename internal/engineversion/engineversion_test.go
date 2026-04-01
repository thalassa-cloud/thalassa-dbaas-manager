/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package engineversion

import (
	"testing"
)

func TestSelectIdentity(t *testing.T) {
	versions := []Version{
		{Identity: "id-16", EngineVersion: "16.0", Enabled: true},
		{Identity: "id-16-10", EngineVersion: "16.10", Enabled: true},
		{Identity: "id-16-10-1", EngineVersion: "16.10.1", Enabled: true},
		{Identity: "id-18-1", EngineVersion: "18.1", Enabled: true},
		{Identity: "id-18-2", EngineVersion: "18.2", Enabled: true},
		{Identity: "id-18-2-3", EngineVersion: "18.2.3", Enabled: true},
		{Identity: "id-17-disabled", EngineVersion: "17.0", Enabled: false},
	}

	tests := []struct {
		name            string
		postgresVersion string
		versions        []Version
		wantEngineVer   string
		wantErr         bool
	}{
		{
			name:            "exact match by engine version",
			postgresVersion: "16.10",
			versions:        versions,
			wantEngineVer:   "16.10",
			wantErr:         false,
		},
		{
			name:            "exact match by identity",
			postgresVersion: "id-18-2",
			versions:        versions,
			wantEngineVer:   "18.2",
			wantErr:         false,
		},
		{
			name:            "major only selects latest 18.x",
			postgresVersion: "18",
			versions:        versions,
			wantEngineVer:   "18.2.3",
			wantErr:         false,
		},
		{
			name:            "major.minor with no exact match selects latest 16.10.x",
			postgresVersion: "16.10",
			versions: []Version{
				{Identity: "id-16-10-1", EngineVersion: "16.10.1", Enabled: true},
				{Identity: "id-16-10-2", EngineVersion: "16.10.2", Enabled: true},
			},
			wantEngineVer: "16.10.2",
			wantErr:       false,
		},
		{
			name:            "whitespace trimmed",
			postgresVersion: "  18  ",
			versions:        versions,
			wantEngineVer:   "18.2.3",
			wantErr:         false,
		},
		{
			name:            "empty input returns error",
			postgresVersion: "",
			versions:        versions,
			wantEngineVer:   "",
			wantErr:         true,
		},
		{
			name:            "only spaces returns error",
			postgresVersion: "   ",
			versions:        versions,
			wantEngineVer:   "",
			wantErr:         true,
		},
		{
			name:            "no match returns error",
			postgresVersion: "99",
			versions:        versions,
			wantEngineVer:   "",
			wantErr:         true,
		},
		{
			name:            "disabled version not selected for semver match",
			postgresVersion: "17",
			versions:        versions,
			wantEngineVer:   "",
			wantErr:         true,
		},
		{
			name:            "single candidate for major returns it",
			postgresVersion: "16",
			versions:        versions,
			wantEngineVer:   "16.10.1",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectIdentity(tt.postgresVersion, tt.versions)
			if (err != nil) != tt.wantErr {
				t.Errorf("SelectIdentity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantEngineVer {
				t.Errorf("SelectIdentity() = %q, want %q", got, tt.wantEngineVer)
			}
		})
	}
}

func TestCompareVersionStrings(t *testing.T) {
	tests := []struct {
		a       string
		b       string
		wantCmp int // negative: a<b, zero: a==b, positive: a>b
	}{
		{"18.2.3", "18.2.3", 0},
		{"18.2.3", "18.2.1", 1},
		{"18.2.1", "18.2.3", -1},
		{"18", "18.1", -1},
		{"18.1", "18", 1},
		{"16.10", "16.10.1", -1},
		{"16.10.1", "16.10", 1},
		{"17", "16", 1},
		{"16", "17", -1},
		{"18.2", "18.10", -1},
		{"18.10", "18.2", 1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := CompareVersionStrings(tt.a, tt.b)
			if tt.wantCmp == 0 {
				if got != 0 {
					t.Errorf("CompareVersionStrings(%q, %q) = %d, want 0", tt.a, tt.b, got)
				}
			} else if (got > 0) != (tt.wantCmp > 0) {
				t.Errorf("CompareVersionStrings(%q, %q) = %d, want %s zero", tt.a, tt.b, got, map[int]string{-1: "negative", 1: "positive"}[tt.wantCmp])
			}
			rev := CompareVersionStrings(tt.b, tt.a)
			if rev != -got {
				t.Errorf("CompareVersionStrings(%q, %q) = %d, expected -%d", tt.b, tt.a, rev, got)
			}
		})
	}
}
