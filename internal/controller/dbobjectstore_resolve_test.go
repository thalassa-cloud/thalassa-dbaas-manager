package controller

import "testing"

func TestDbObjectStoreSpecDrift(t *testing.T) {
	tests := []struct {
		name      string
		desired   string
		fetched   string
		wantDrift bool
	}{
		{name: "match", desired: "os-1", fetched: "os-1", wantDrift: false},
		{name: "both empty", desired: "", fetched: "", wantDrift: false},
		{name: "attach", desired: "os-2", fetched: "", wantDrift: true},
		{name: "change", desired: "os-b", fetched: "os-a", wantDrift: true},
		{name: "detach not reconciled", desired: "", fetched: "os-1", wantDrift: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dbObjectStoreSpecDrift(tt.desired, tt.fetched); got != tt.wantDrift {
				t.Fatalf("dbObjectStoreSpecDrift(%q, %q) = %v, want %v", tt.desired, tt.fetched, got, tt.wantDrift)
			}
		})
	}
}
