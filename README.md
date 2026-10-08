# csheet

A simple Go CLI tool that generates a _contact sheet_ (a thumbnail grid) from a video. It grabs 16 evenly spaced frames, stamps each one with a timestamp (`HH:MM:SS`), and arranges them into a single 4×4 JPG image.

### Features

- Captures 16 evenly spaced frames (from 1% to 99% of the video's duration).
- `HH:MM:SS` timestamp label at the bottom of every frame.
- Outputs one JPG image with a 4-column grid on a white background.
- Each frame is 480 px wide, with a 5 px margin.
- Temporary files are cleaned up automatically.

### Requirements

- [Go](https://go.dev/dl/) 1.22 or newer.
- `ffmpeg` and `ffprobe` on your `PATH` (version 7.0+ recommended, since the tool uses the `-/filter_complex` option).
- _Optional:_ the Segoe UI Semibold font at `~/.local/share/fonts/truetype/segoeui/SegoeUI-Semibold.ttf`. If it is missing, ffmpeg falls back to its default font.

### Installation

```bash
git clone git@github.com:egoletter/csheet.git
cd csheet
go build -o csheet main.go
```

### Usage

```bash
./csheet --video=/path/to/video.mp4
```

Options:

| Option    | Description                                  |
| --------- | -------------------------------------------- |
| `--video` | Path to the source video file **(required)** |

Example result:

<img src="./example_result.jpg" alt="Example result" width="540" />

### Output

The image is saved next to the video with a `.jpg` extension. `.mp4` and `.mkv` extensions are replaced with `.jpg`. For any other extension, `.jpg` is appended to the file name so the source file is never overwritten. An existing `.jpg` with the same name will be overwritten.

### Customization

There are no command-line flags for layout yet. To customize the output, edit the constants at the top of `main.go` and rebuild.

```go
const (
	frameWidth = 480 // width of each frame (px)
	frameCount = 16  // number of frames to capture
	sheetCols  = 4   // images per row (columns)
	margin     = 5   // spacing between images (px)
)
```

| Constant     | Purpose                              |
| ------------ | ------------------------------------ |
| `sheetCols`  | Images per row                       |
| `frameCount` | Total images in the sheet            |
| `frameWidth` | Width of each thumbnail              |
| `margin`     | Spacing between thumbnails and edges |

The number of rows is calculated automatically from `frameCount` and `sheetCols`. For example, `frameCount = 20` and `sheetCols = 5` gives a 5×4 grid. After changing a value, rebuild with `go build -o csheet main.go`.

Other things you can change in the code:

- **Capture time range**: `startSeek` and `endSeek` in `createLabeledFrames` (default `0.01` and `0.99` of the duration).
- **Timestamp label size, color and position**: the `drawtext` filter string in `createLabeledFrames` (`fontsize=18`, `fontcolor=white@0.5`, etc.).
- **Font**: the path in `getFontPath`.
- **Background color**: `color=c=white` in `createFilterScript`.
- **Video extensions that get replaced**: the `extRe` regular expression (default `mp4` and `mkv`).

### How it works

1. Checks that `ffmpeg` and `ffprobe` are available.
2. Reads the video duration with `ffprobe`.
3. Captures 16 frames, scaled to 480 px wide and labeled with their timestamps.
4. Combines the frames into a 4×4 grid using ffmpeg's `overlay` filter.
5. Saves the result and removes the temporary folder.
