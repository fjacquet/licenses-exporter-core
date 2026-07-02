package licenses_core

import "testing"

func labelValue(s Sample, key string) (string, bool) {
	for _, l := range s.Labels {
		if l.Key == key {
			return l.Value, true
		}
	}
	return "", false
}

func TestSeatSampleHasCanonicalLabelKeys(t *testing.T) {
	s := SeatSample(MetricSeatsTotal, "vmware", "vSphere_ENT+", "cpuPackage", "vcsa01", 512)
	if s.Name != "license_seats_total" {
		t.Fatalf("name = %q", s.Name)
	}
	if s.Value != 512 {
		t.Fatalf("value = %v", s.Value)
	}
	// Labels must be sorted by key: instance, product, unit, vendor.
	wantKeys := []string{"instance", "product", "unit", "vendor"}
	if len(s.Labels) != len(wantKeys) {
		t.Fatalf("label count = %d, want %d", len(s.Labels), len(wantKeys))
	}
	for i, k := range wantKeys {
		if s.Labels[i].Key != k {
			t.Fatalf("label[%d].Key = %q, want %q", i, s.Labels[i].Key, k)
		}
	}
	if v, _ := labelValue(s, "vendor"); v != "vmware" {
		t.Fatalf("vendor = %q", v)
	}
}

func TestUpSampleUsesVendorInstanceOnly(t *testing.T) {
	s := UpSample("microsoft", "tenant-a", false)
	if s.Name != "license_up" || s.Value != 0 {
		t.Fatalf("got %q=%v", s.Name, s.Value)
	}
	if len(s.Labels) != 2 {
		t.Fatalf("up label count = %d, want 2", len(s.Labels))
	}
	if _, ok := labelValue(s, "product"); ok {
		t.Fatal("up must not carry a product label")
	}
}

// TestMetricLabelKeysAreLocked guards schema identity across every vendor: if
// a constructor's label set changes, this fails before any vendor ships.
// Label order here mirrors the constructors' canonical sorted-by-key order
// (instance, product, unit, vendor) — see sample.go / metrics.go.
func TestMetricLabelKeysAreLocked(t *testing.T) {
	cases := []struct {
		name   string
		sample Sample
		want   []string
	}{
		{"seats", SeatSample(MetricSeatsTotal, "microsoft", "SPE_E5", "users", "t-a", 1), []string{"instance", "product", "unit", "vendor"}},
		{"exp", ExpirationSample("microsoft", "SPE_E5", "t-a", 1), []string{"instance", "product", "vendor"}},
	}
	for _, c := range cases {
		var got []string
		for _, l := range c.sample.Labels {
			got = append(got, l.Key)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: label keys %v, want %v", c.name, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("%s: label[%d]=%q want %q", c.name, i, got[i], c.want[i])
			}
		}
	}
}
