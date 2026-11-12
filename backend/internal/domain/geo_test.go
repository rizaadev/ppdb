package domain

import "testing"

func TestHaversineMeters(t *testing.T) {
	if d := HaversineMeters(-6.2088, 106.8456, -6.2088, 106.8456); d != 0 {
		t.Fatalf("identical coordinates distance = %d, want 0", d)
	}

	// Jakarta Monas (-6.175392, 106.827153) to Istiqlal (-6.170170, 106.831390) is ~745m.
	d := HaversineMeters(-6.175392, 106.827153, -6.170170, 106.831390)
	if d < 650 || d > 850 {
		t.Fatalf("unexpected Monas-Istiqlal distance: %d meters", d)
	}
}

