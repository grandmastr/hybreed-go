package format

import "testing"

func TestPace(t *testing.T) {
	cases := map[int]string{
		0:   "0:00",
		275: "4:35",
		60:  "1:00",
		305: "5:05",
	}
	for in, want := range cases {
		if got := Pace(in); got != want {
			t.Errorf("Pace(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestClock(t *testing.T) {
	cases := map[int]string{
		0:    "0:00",
		1710: "28:30", // 28:30 tempo run
		5040: "1:24:00",
		90:   "1:30",
	}
	for in, want := range cases {
		if got := Clock(in); got != want {
			t.Errorf("Clock(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestKm(t *testing.T) {
	cases := map[int]float64{
		6200:  6.2,
		16400: 16.4,
		0:     0,
		8000:  8.0,
	}
	for in, want := range cases {
		if got := Km(in); got != want {
			t.Errorf("Km(%d) = %v, want %v", in, got, want)
		}
	}
}
