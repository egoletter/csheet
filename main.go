package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	frameWidth = 480
	frameCount = 16
	sheetCols  = 4
	margin     = 5
)

// formatTime formats seconds as HH:MM:SS
func formatTime(sec float64) string {
	total := int(math.Floor(sec))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func run(name string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

func getVideoDuration(videoPath string) float64 {
	out, _, _ := run("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", videoPath)
	d, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
	if err != nil {
		return 0
	}
	return d
}

func getFontPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".local/share/fonts/truetype/segoeui/SegoeUI-Semibold.ttf")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func captureFrame(seek float64, vinput, filter, outpath string) error {
	_, stderr, err := run("ffmpeg", "-y", "-ss", strconv.FormatFloat(seek, 'f', 3, 64),
		"-i", vinput, "-vf", filter, "-frames:v", "1", "-q:v", "2", outpath)
	if err != nil {
		fmt.Fprintln(os.Stderr, stderr)
		return fmt.Errorf("error capturing frame: %w", err)
	}
	return nil
}

func createLabeledFrames(duration float64, vinput, tempdir, fontfile string) ([]string, error) {
	startSeek := duration * 0.01
	endSeek := duration * 0.99
	fontOpt := ""
	if fontfile != "" {
		fontOpt = ":fontfile=" + fontfile
	}

	frames := make([]string, 0, frameCount)
	for i := range frameCount {
		seek := startSeek + (endSeek-startSeek)*float64(i)/float64(frameCount-1)
		framePath := filepath.Join(tempdir, fmt.Sprintf("frame_%02d.jpg", i))
		labelTime := strings.ReplaceAll(formatTime(seek), ":", `\:`)

		filter := fmt.Sprintf("scale=%d:-1,drawtext=text='%s'%s", frameWidth, labelTime, fontOpt)
		filter += ":fontsize=18:fontcolor=white@0.5:bordercolor=black@0.5:borderw=2:x=(w-tw)/2:y=h-th-14"

		if err := captureFrame(seek, vinput, filter, framePath); err != nil {
			return nil, err
		}
		fmt.Printf("[ffmpeg] Captured frame %d/%d\n", i+1, frameCount)
		frames = append(frames, framePath)
	}
	return frames, nil
}

func getFrameSize(framePath string) (int, int, error) {
	fmt.Printf("[ffprobe] Opening frame %s\n", framePath)
	out, stderr, err := run("ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", framePath)
	if err != nil {
		return 0, 0, fmt.Errorf("error opening frame: %w %s", err, stderr)
	}
	var data struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil || len(data.Streams) == 0 {
		return 0, 0, fmt.Errorf("invalid ffprobe output")
	}
	return data.Streams[0].Width, data.Streams[0].Height, nil
}

func createFilterScript(n, cols, fw, fh int) string {
	cw := cols*fw + (cols+1)*margin
	rows := (n + cols - 1) / cols
	ch := rows*fh + (rows+1)*margin

	var sb strings.Builder
	fmt.Fprintf(&sb, "color=c=white:s=%dx%d[base];", cw, ch)
	prev := "base"
	for i := range n {
		row, col := i/cols, i%cols
		x := margin + col*(fw+margin)
		y := margin + row*(fh+margin)
		label := fmt.Sprintf("afterFrame%d", i)
		if i == n-1 {
			label = "final"
		}
		fmt.Fprintf(&sb, "[%s][%d:v]overlay=%d:%d[%s];", prev, i, x, y, label)
		prev = label
	}
	return strings.TrimSuffix(sb.String(), ";")
}

var extRe = regexp.MustCompile(`\.(mkv|mp4)$`)

func createContactSheet(videoPath string) (string, error) {
	tempdir, err := os.MkdirTemp("", "tmp-frames-")
	if err != nil {
		return "", err
	}
	defer func() {
		fmt.Printf("[info] Removing temporary path: %s\n", tempdir)
		os.RemoveAll(tempdir)
	}()

	fontfile := getFontPath()

	fmt.Println("[ffprobe] Getting video duration")
	duration := getVideoDuration(videoPath)

	frames, err := createLabeledFrames(duration, videoPath, tempdir, fontfile)
	if err != nil {
		return "", err
	}
	fw, fh, err := getFrameSize(frames[0])
	if err != nil {
		return "", err
	}

	fmt.Println("[info] Creating filter complex script")
	script := createFilterScript(len(frames), sheetCols, fw, fh)
	scriptFile := filepath.Join(tempdir, "filter.txt")
	if err := os.WriteFile(scriptFile, []byte(script), 0o644); err != nil {
		return "", err
	}

	outpath := extRe.ReplaceAllString(videoPath, ".jpg")
	if outpath == videoPath {
		outpath += ".jpg" // avoid overwriting the source file
	}
	fmt.Printf("[info] Output path: %s\n", outpath)

	args := []string{}
	for _, f := range frames {
		args = append(args, "-i", f)
	}
	args = append(args, "-y", "-/filter_complex", scriptFile, "-loglevel", "warning",
		"-map", "[final]", "-frames:v", "1", "-q:v", "2", outpath)

	fmt.Println("[ffmpeg] Creating frame output")
	if _, stderr, err := run("ffmpeg", args...); err != nil {
		fmt.Fprintln(os.Stderr, stderr)
		return "", fmt.Errorf("ffmpeg failed: %w", err)
	}

	fmt.Printf("[ffmpeg] Frame output saved to: %s\n", outpath)
	return outpath, nil
}

func checkTool(name string) error {
	fmt.Printf("[info] Checking %s\n", name)
	out, stderr, err := run(name, "-version")
	if err != nil {
		if stderr == "" {
			stderr = err.Error()
		}
		return fmt.Errorf("%s", strings.TrimSpace(stderr))
	}
	fmt.Printf("[info] %s\n", strings.SplitN(out, "\n", 2)[0])
	return nil
}

func fail(msg string) {
	fmt.Fprintf(os.Stderr, "\x1b[1;91merror\x1b[0m %s\n", msg)
	os.Exit(1)
}

func main() {
	video := flag.String("video", "", "path to source file [required]")
	flag.Usage = func() {
		fmt.Print("Usage: csheet [options]\n\nOptions:\n--video=...        path to source file [required]\n")
	}
	flag.Parse()

	if *video == "" {
		fail("video source is required")
	}

	videoPath, err := filepath.Abs(*video)
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("[info] Processing video: %s\n", videoPath)

	if _, err := os.Stat(videoPath); err != nil {
		fail(err.Error())
	}

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if err := checkTool(tool); err != nil {
			fail(err.Error())
		}
	}

	if _, err := createContactSheet(videoPath); err != nil {
		fail(err.Error())
	}
}
