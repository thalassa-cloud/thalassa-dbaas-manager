package helpers

import (
	"math"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestQuantityFromDecimalGigabytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		gb   float64
		want string // canonical quantity string for comparison
	}{
		{name: "zero", gb: 0, want: "0"},
		{name: "negative", gb: -1, want: "0"},
		{name: "small fractional gb", gb: 0.000001, want: "1000"}, // 1e-6 G = 1000 bytes (decimal SI)
		{name: "one gb", gb: 1, want: "1G"},
		{name: "fractional", gb: 1.5, want: "1500000000"},
		{name: "nan", gb: math.NaN(), want: "0"},
		{name: "inf", gb: math.Inf(1), want: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := QuantityFromDecimalGigabytes(tt.gb)
			wantQ := resource.MustParse(tt.want)
			if !got.Equal(wantQ) {
				t.Fatalf("QuantityFromDecimalGigabytes(%v) = %q (%v), want equal to %q (%v)", tt.gb, got.String(), got, wantQ.String(), wantQ)
			}
		})
	}
}
