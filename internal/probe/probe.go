// Package probe runs ffprobe over a presigned GET URL and validates the
// resulting metadata against a profile's constraint fields.
package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"mediaflow/internal/config"
)

type Result struct {
	Asset   AssetInfo `json:"asset"`
	Video   *Stream   `json:"video,omitempty"`
	Audio   *Stream   `json:"audio,omitempty"`
	Reasons []Reason  `json:"reasons"`
	OK      bool      `json:"ok"`
}

type AssetInfo struct {
	ObjectKey string `json:"object_key"`
	SizeBytes int64  `json:"size_bytes"`
	Container string `json:"container"`
}

type Stream struct {
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
	Codec           string  `json:"codec,omitempty"`
	BitrateKbps     int     `json:"bitrate_kbps,omitempty"`
	Framerate       float64 `json:"framerate,omitempty"`
	Channels        int     `json:"channels,omitempty"`
	SampleRateHz    int     `json:"sample_rate_hz,omitempty"`
}

type Reason struct {
	Code   string `json:"code"`
	Limit  any    `json:"limit,omitempty"`
	Actual any    `json:"actual,omitempty"`
}

// ffprobe -print_format json -show_format -show_streams output (subset).
type ffprobeOutput struct {
	Format  ffprobeFormat   `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeFormat struct {
	Duration   string `json:"duration"`
	Size       string `json:"size"`
	BitRate    string `json:"bit_rate"`
	FormatName string `json:"format_name"`
}

type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
	RFrameRate string `json:"r_frame_rate"`
	Channels   int    `json:"channels"`
	SampleRate string `json:"sample_rate"`
}

const probeTimeout = 30 * time.Second

func Probe(ctx context.Context, presignedGetURL, objectKey string, profile *config.Profile) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		presignedGetURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var raw ffprobeOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	res := buildResult(raw, objectKey)
	res.Reasons = checkConstraints(res, profile)
	res.OK = len(res.Reasons) == 0
	return res, nil
}

func buildResult(raw ffprobeOutput, objectKey string) *Result {
	res := &Result{
		Asset: AssetInfo{
			ObjectKey: objectKey,
			SizeBytes: parseInt64(raw.Format.Size),
			Container: pickContainer(raw.Format.FormatName),
		},
		Reasons: []Reason{},
	}

	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if res.Video != nil {
				continue
			}
			duration := parseFloat(s.Duration)
			if duration == 0 {
				duration = parseFloat(raw.Format.Duration)
			}
			res.Video = &Stream{
				DurationSeconds: duration,
				Width:           s.Width,
				Height:          s.Height,
				Codec:           s.CodecName,
				BitrateKbps:     bitrateKbps(s.BitRate, raw.Format.BitRate),
				Framerate:       parseFraction(s.RFrameRate),
			}
		case "audio":
			if res.Audio != nil {
				continue
			}
			res.Audio = &Stream{
				Codec:        s.CodecName,
				BitrateKbps:  bitrateKbps(s.BitRate, ""),
				Channels:     s.Channels,
				SampleRateHz: int(parseInt64(s.SampleRate)),
			}
		}
	}

	return res
}

func checkConstraints(res *Result, profile *config.Profile) []Reason {
	reasons := []Reason{}

	if res.Video == nil {
		reasons = append(reasons, Reason{Code: "no_video_stream"})
		return reasons
	}

	v := res.Video

	if profile.MaxDurationSeconds > 0 && v.DurationSeconds > float64(profile.MaxDurationSeconds) {
		reasons = append(reasons, Reason{
			Code:   "duration_exceeded",
			Limit:  profile.MaxDurationSeconds,
			Actual: v.DurationSeconds,
		})
	}
	if profile.MinWidth > 0 && v.Width < profile.MinWidth {
		reasons = append(reasons, Reason{
			Code:   "width_too_low",
			Limit:  profile.MinWidth,
			Actual: v.Width,
		})
	}
	if profile.MinHeight > 0 && v.Height < profile.MinHeight {
		reasons = append(reasons, Reason{
			Code:   "height_too_low",
			Limit:  profile.MinHeight,
			Actual: v.Height,
		})
	}
	if profile.MaxWidth > 0 && v.Width > profile.MaxWidth {
		reasons = append(reasons, Reason{
			Code:   "width_too_high",
			Limit:  profile.MaxWidth,
			Actual: v.Width,
		})
	}
	if profile.MaxHeight > 0 && v.Height > profile.MaxHeight {
		reasons = append(reasons, Reason{
			Code:   "height_too_high",
			Limit:  profile.MaxHeight,
			Actual: v.Height,
		})
	}
	if len(profile.AllowedCodecs) > 0 && !contains(profile.AllowedCodecs, v.Codec) {
		reasons = append(reasons, Reason{
			Code:   "codec_not_allowed",
			Limit:  profile.AllowedCodecs,
			Actual: v.Codec,
		})
	}

	return reasons
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ffprobe emits framerate as "30000/1001"; evaluate the fraction.
func parseFraction(s string) float64 {
	if s == "" {
		return 0
	}
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 1 {
		return parseFloat(parts[0])
	}
	num := parseFloat(parts[0])
	den := parseFloat(parts[1])
	if den == 0 {
		return 0
	}
	return num / den
}

// ffprobe bit_rate fields are bps strings; convert to kbps with format-level fallback.
func bitrateKbps(stream, format string) int {
	bps := parseInt64(stream)
	if bps == 0 {
		bps = parseInt64(format)
	}
	if bps == 0 {
		return 0
	}
	return int(bps / 1000)
}

// ffprobe emits comma-joined container names like "mov,mp4,m4a,3gp,3g2,mj2"; take the first.
func pickContainer(formatName string) string {
	if formatName == "" {
		return ""
	}
	if i := strings.Index(formatName, ","); i >= 0 {
		return formatName[:i]
	}
	return formatName
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
