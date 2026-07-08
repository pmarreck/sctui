package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/gopxl/beep"
)

const ffmpegHLSOutputSampleRate = beep.SampleRate(44100)

var urlForErrorRedaction = regexp.MustCompile(`https?://\S+`)

// HLSDecoder turns a SoundCloud HLS playlist into seekable decoded audio; the
// production adapter shells out to ffmpeg while tests inject a fake decoder.
type HLSDecoder interface {
	Decode(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error)
}

// FFmpegRunner is the subprocess port used by FFmpegHLSDecoder, kept small so
// tests can assert command construction without spawning ffmpeg.
type FFmpegRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, string, error)
}

type execFFmpegRunner struct{}

func (execFFmpegRunner) Run(ctx context.Context, name string, args ...string) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return stdout.Bytes(), stderr.String(), err
}

// FFmpegHLSDecoder decodes AAC/Opus HLS through ffmpeg into stereo 44.1kHz
// s16le PCM, then wraps the bytes in an in-memory Beep streamer.
type FFmpegHLSDecoder struct {
	command    string
	runner     FFmpegRunner
	sampleRate beep.SampleRate
}

// FFmpegOption configures the ffmpeg-backed HLS decoder for tests or alternate
// installations.
type FFmpegOption func(*FFmpegHLSDecoder)

func WithFFmpegRunner(r FFmpegRunner) FFmpegOption {
	return func(d *FFmpegHLSDecoder) {
		if r != nil {
			d.runner = r
		}
	}
}

func WithFFmpegCommand(command string) FFmpegOption {
	return func(d *FFmpegHLSDecoder) {
		if command != "" {
			d.command = command
		}
	}
}

func NewFFmpegHLSDecoder(opts ...FFmpegOption) *FFmpegHLSDecoder {
	d := &FFmpegHLSDecoder{
		command:    "ffmpeg",
		runner:     execFFmpegRunner{},
		sampleRate: ffmpegHLSOutputSampleRate,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *FFmpegHLSDecoder) Decode(ctx context.Context, streamURL string) (beep.StreamSeekCloser, beep.Format, error) {
	if strings.TrimSpace(streamURL) == "" {
		return nil, beep.Format{}, fmt.Errorf("stream URL cannot be empty")
	}
	runner := d.runner
	if runner == nil {
		runner = execFFmpegRunner{}
	}
	command := d.command
	if command == "" {
		command = "ffmpeg"
	}
	sampleRate := d.sampleRate
	if sampleRate <= 0 {
		sampleRate = ffmpegHLSOutputSampleRate
	}

	pcm, stderr, err := runner.Run(ctx, command,
		"-hide_banner",
		"-loglevel", "error",
		"-i", streamURL,
		"-f", "s16le",
		"-ac", "2",
		"-ar", fmt.Sprintf("%d", sampleRate),
		"pipe:1",
	)
	if err != nil {
		stderr = sanitizeFFmpegStderr(stderr)
		if stderr != "" {
			return nil, beep.Format{}, fmt.Errorf("ffmpeg HLS decode failed: %w: %s", err, stderr)
		}
		return nil, beep.Format{}, fmt.Errorf("ffmpeg HLS decode failed: %w", err)
	}

	streamer, format, err := NewPCMStreamSeekCloser(pcm, sampleRate)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("ffmpeg HLS decode produced unusable PCM: %w", err)
	}
	return streamer, format, nil
}

func sanitizeFFmpegStderr(stderr string) string {
	return strings.TrimSpace(urlForErrorRedaction.ReplaceAllString(stderr, "[url redacted]"))
}
