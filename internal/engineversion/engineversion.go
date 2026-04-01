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
	"fmt"
	"strconv"
	"strings"
)

// Version represents an engine version with identity, version string, and enabled flag.
// Callers can map from provider types (e.g. dbaas.DbClusterEngineVersion) into this shape.
type Version struct {
	Identity      string
	EngineVersion string
	Enabled       bool
}

// SelectIdentity returns the identity of the best-matching enabled version for the given
// postgresVersion. It supports:
//   - Exact match by identity or engine version string (e.g. "16.10").
//   - Semver-style patterns: "18" selects the latest enabled 18.x; "16.10" selects the latest 16.10.x.
//
// Only Enabled versions are considered. If no match is found, an error is returned.
func SelectIdentity(postgresVersion string, versions []Version) (string, error) {
	postgresVersion = strings.TrimSpace(postgresVersion)
	if postgresVersion == "" {
		return "", fmt.Errorf("postgres version must not be empty")
	}

	// Exact match by identity or by engine version string
	for _, v := range versions {
		if !v.Enabled {
			continue
		}
		if v.Identity == postgresVersion || strings.EqualFold(v.EngineVersion, postgresVersion) {
			return v.EngineVersion, nil
		}
	}

	// Semver-style match: "18" -> latest 18.x, "16.10" -> latest 16.10.x
	var candidates []Version
	prefix := postgresVersion + "."
	for _, v := range versions {
		if !v.Enabled {
			continue
		}
		if v.EngineVersion == postgresVersion {
			candidates = append(candidates, v)
			continue
		}
		if strings.HasPrefix(v.EngineVersion, prefix) {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no enabled engine version found for %q", postgresVersion)
	}
	latest := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if CompareVersionStrings(candidates[i].EngineVersion, latest.EngineVersion) > 0 {
			latest = candidates[i]
		}
	}
	return latest.EngineVersion, nil
}

// CompareVersionStrings compares two version strings (e.g. "18.2.3", "16.10").
// Returns negative if a < b, zero if a == b, positive if a > b.
// Useful for tests and for consumers that need to sort versions.
func CompareVersionStrings(a, b string) int {
	partsA := parseVersionParts(a)
	partsB := parseVersionParts(b)
	for i := 0; i < len(partsA) || i < len(partsB); i++ {
		var va, vb int
		if i < len(partsA) {
			va = partsA[i]
		}
		if i < len(partsB) {
			vb = partsB[i]
		}
		if va != vb {
			return va - vb
		}
	}
	return 0
}

func parseVersionParts(s string) []int {
	var parts []int
	for _, seg := range strings.Split(s, ".") {
		seg = strings.TrimSpace(seg)
		n, err := strconv.Atoi(seg)
		if err != nil {
			n = 0
		}
		parts = append(parts, n)
	}
	return parts
}
