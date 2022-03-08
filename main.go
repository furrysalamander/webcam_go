package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const targetWidth = 80

// The height must be a multiple of two.
const targetHeight = ((targetWidth / 16 * 9) / 2) * 2

var renderStream = make(chan string)

var ffmpegCommands = []string{
	"-framerate", "60",
	"-video_size", "320x180",
	"-input_format", "mjpeg",
	"-i", "/dev/video0",
	"-f", "rawvideo",
	"-vf", fmt.Sprintf("scale=%dx%d,setsar=1:1", targetWidth, targetHeight),
	"-pix_fmt", "rgb24",
	"pipe:",
}

// RenderWebcam renders the default webcam to the terminal.
func RenderWebcam() {
	ffmpegProcess := exec.Command(FfmpegBinary, ffmpegCommands...)

	stdout, _ := ffmpegProcess.StdoutPipe()

	ffmpegProcess.Start()
	defer ffmpegProcess.Wait()

	ffmpegStdoutStream := bufio.NewReader(stdout)

	RenderByteStream(ffmpegStdoutStream, targetHeight, targetWidth, 0, 0)
}

// This is a temporary hack for testing multiple simultaneous video streams.
func RenderRTSP() {
	var rtspFlags = []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://wowzaec2demo.streamlock.net/vod/mp4:BigBuckBunny_115k.mp4",
		"-f", "rawvideo",
		"-vf", fmt.Sprintf("scale=%dx%d,setsar=1:1", targetWidth, targetHeight),
		"-pix_fmt", "rgb24",
		"pipe:",
	}

	ffmpegProcess := exec.Command(FfmpegBinary, rtspFlags...)

	stdout, _ := ffmpegProcess.StdoutPipe()

	ffmpegProcess.Start()
	defer ffmpegProcess.Wait()

	ffmpegStdoutStream := bufio.NewReader(stdout)

	RenderByteStream(ffmpegStdoutStream, targetHeight, targetWidth, 0, 30)
}

// RenderByteStream renders an arbitrary bytes buffer to the terminal.  It will render it to screen at given x and y offset.
func RenderByteStream(buffer *bufio.Reader, height, width, offsetX, offsetY uint) {

	// The size of the static buffer for holding raw frame data
	bufferSize := targetHeight * targetWidth * 3

	// The buffer for holding the raw RGB values for the current frame
	frameData := make([]byte, bufferSize)

	// For holding the formatted escape sequence
	var sb strings.Builder

	for {
		// Start by moving the cursor to the appropriate coordinates
		sb.WriteString(
			fmt.Sprintf("\033[%d;%dH", offsetY, offsetX),
		)

		// If there are any extra frames, drop them.  It's better to have dropped frames
		// than to lag an increasing amount over time.
		for buffer.Buffered() > bufferSize*2 {
			buffer.Discard(bufferSize)
		}

		// Fill the frameData buffer with a single frame's worth of pixel information.
		io.ReadFull(buffer, frameData)

		// Iterate through the frame two rows at a time.  This is necessary because each
		// character renders two pixels.
		for rowIndex := 0; rowIndex < targetHeight; rowIndex += 2 {
			for columnIndex := 0; columnIndex < targetWidth; columnIndex++ {
				// Find the correct offset in the frame data for the current pixel
				topPixelStart := ((rowIndex * targetWidth) + columnIndex) * 3
				bottomPixelStart := (((rowIndex + 1) * targetWidth) + columnIndex) * 3
				// Populate the final buffer with a single formatted character.
				sb.WriteString(fmt.Sprintf(
					"\033[48;2;%d;%d;%dm\033[38;2;%d;%d;%dm▄",
					frameData[topPixelStart],
					frameData[topPixelStart+1],
					frameData[topPixelStart+2],
					frameData[bottomPixelStart],
					frameData[bottomPixelStart+1],
					frameData[bottomPixelStart+2],
				))
			}
			// Move the cursor down a single row and back to the starting column.
			sb.WriteString(
				fmt.Sprintf("\033[B\033[%dD", targetWidth),
			)
		}
		// Reset the output back to standard colors.
		sb.WriteString("\033[m")

		// Hand off the formatted string to the render thread.
		renderStream <- sb.String()

		// Wipe the formatted string so we're ready for the next frame.
		sb.Reset()
	}
}

func DisplayRenderThread() {
	for {
		fmt.Print(<-renderStream)
	}
}

func main() {
	go RenderWebcam()

	// Ensure that the terminal has been wiped
	renderStream <- "\033[H\033[2J"

	RenderRTSP()
}

func init() {
	go DisplayRenderThread()
}
