package probe

import (
	"testing"

	"mediaflow/internal/config"
)

func TestCheckConstraints(t *testing.T) {
	shopTrailer := &config.Profile{
		MaxDurationSeconds: 45,
		MinWidth:           1280,
		MinHeight:          720,
		AllowedCodecs:      []string{"h264", "hevc"},
	}

	cases := []struct {
		name    string
		profile *config.Profile
		video   *Stream
		wantOK  bool
		wantCodes []string
	}{
		{
			name:    "passes shop-trailer",
			profile: shopTrailer,
			video:   &Stream{DurationSeconds: 30, Width: 1920, Height: 1080, Codec: "h264"},
			wantOK:  true,
		},
		{
			name:    "duration over limit",
			profile: shopTrailer,
			video:   &Stream{DurationSeconds: 67.4, Width: 1920, Height: 1080, Codec: "h264"},
			wantOK:  false,
			wantCodes: []string{"duration_exceeded"},
		},
		{
			name:    "below min dimensions",
			profile: shopTrailer,
			video:   &Stream{DurationSeconds: 30, Width: 854, Height: 480, Codec: "h264"},
			wantOK:  false,
			wantCodes: []string{"width_too_low", "height_too_low"},
		},
		{
			name:    "disallowed codec",
			profile: shopTrailer,
			video:   &Stream{DurationSeconds: 30, Width: 1920, Height: 1080, Codec: "vp9"},
			wantOK:  false,
			wantCodes: []string{"codec_not_allowed"},
		},
		{
			name:    "unset constraints permit anything",
			profile: &config.Profile{},
			video:   &Stream{DurationSeconds: 9999, Width: 16, Height: 16, Codec: "wmv3"},
			wantOK:  true,
		},
		{
			name:    "no video stream",
			profile: shopTrailer,
			video:   nil,
			wantOK:  false,
			wantCodes: []string{"no_video_stream"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := &Result{Video: tc.video}
			reasons := checkConstraints(res, tc.profile)
			ok := len(reasons) == 0
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want=%v reasons=%+v", ok, tc.wantOK, reasons)
			}
			if len(reasons) != len(tc.wantCodes) {
				t.Fatalf("got %d reasons want %d: %+v", len(reasons), len(tc.wantCodes), reasons)
			}
			for i, code := range tc.wantCodes {
				if reasons[i].Code != code {
					t.Errorf("reason[%d].Code=%q want %q", i, reasons[i].Code, code)
				}
			}
		})
	}
}

func TestParseFraction(t *testing.T) {
	cases := map[string]float64{
		"":          0,
		"30":        30,
		"30000/1001": 30000.0 / 1001.0,
		"30/0":      0,
	}
	for in, want := range cases {
		got := parseFraction(in)
		if got != want {
			t.Errorf("parseFraction(%q) = %v want %v", in, got, want)
		}
	}
}

func TestPickContainer(t *testing.T) {
	cases := map[string]string{
		"":                            "",
		"mp4":                         "mp4",
		"mov,mp4,m4a,3gp,3g2,mj2":     "mov",
	}
	for in, want := range cases {
		if got := pickContainer(in); got != want {
			t.Errorf("pickContainer(%q) = %q want %q", in, got, want)
		}
	}
}
