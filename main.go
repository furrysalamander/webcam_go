package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

const targetWidth = 80

// The height must be a multiple of two.
const targetHeight = ((targetWidth / 16 * 9) / 2) * 2

var renderStream = make(chan string)
var keyboardEvents = make(chan string)

// RenderMinecraftDirectly renders the Minecraft X11 screen directly to the terminal
func RenderMinecraftDirectly() {
	var x11GrabFlags = []string{
		"-f", "x11grab",
		"-video_size", "1280x720",
		"-i", ":44",
		"-f", "rawvideo",
		"-vf", fmt.Sprintf("scale=%dx%d,setsar=1:1", targetWidth, targetHeight),
		"-pix_fmt", "rgb24",
		"pipe:",
	}

	ffmpegProcess := exec.Command(FfmpegBinary, x11GrabFlags...)

	stdout, _ := ffmpegProcess.StdoutPipe()

	ffmpegProcess.Start()
	defer ffmpegProcess.Wait()

	ffmpegStdoutStream := bufio.NewReader(stdout)

	RenderByteStream(ffmpegStdoutStream, targetHeight, targetWidth, 0, 0)
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

// CaptureKeyboardInput sets up the terminal for raw input and captures keystrokes
func CaptureKeyboardInput() {
	// Put terminal into raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Println("Failed to set terminal to raw mode:", err)
		return
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Buffer for reading individual keystrokes
	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			continue
		}
		
		// Special handling for escape sequences
		if buf[0] == 27 { // ESC
			escBuf := make([]byte, 2)
			_, err := os.Stdin.Read(escBuf)
			if err == nil {
				// Handle arrow keys and other special keys
				if escBuf[0] == 91 { // [
					keyboardEvents <- fmt.Sprintf("SPECIAL_%c", escBuf[1])
				}
			} else {
				keyboardEvents <- "ESC"
			}
		} else {
			// Regular key
			keyboardEvents <- string(buf)
		}
	}
}

// SendKeyboardToMinecraft forwards captured keyboard input to the Minecraft instance
func SendKeyboardToMinecraft() {
	for {
		key := <-keyboardEvents
		var cmd *exec.Cmd
		
		// Map keys to xdotool commands
		switch key {
		case "w":
			cmd = exec.Command("xdotool", "key", "w")
		case "a":
			cmd = exec.Command("xdotool", "key", "a")
		case "s":
			cmd = exec.Command("xdotool", "key", "s")
		case "d":
			cmd = exec.Command("xdotool", "key", "d")
		case " ":
			cmd = exec.Command("xdotool", "key", "space")
		case "SPECIAL_A": // Up arrow
			cmd = exec.Command("xdotool", "key", "Up")
		case "SPECIAL_B": // Down arrow
			cmd = exec.Command("xdotool", "key", "Down")
		case "SPECIAL_C": // Right arrow
			cmd = exec.Command("xdotool", "key", "Right")
		case "SPECIAL_D": // Left arrow
			cmd = exec.Command("xdotool", "key", "Left")
		case "ESC":
			cmd = exec.Command("xdotool", "key", "Escape")
		case "\r":
			cmd = exec.Command("xdotool", "key", "Return")
		case "e":
			cmd = exec.Command("xdotool", "key", "e")
		case "q":
			cmd = exec.Command("xdotool", "key", "q")
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			cmd = exec.Command("xdotool", "key", key)
		case "b":
			return
		default:
			SendMouseClicksToMinecraft()
			// Other keys can be mapped as needed
			continue
		}
		
		if cmd != nil {
			cmd.Env = append(os.Environ(), "DISPLAY=:44")
			cmd.Run()
		}
	}
}

// SendMouseClicksToMinecraft simulates mouse clicks in the Minecraft window
func SendMouseClicksToMinecraft() {
	leftClickCmd := exec.Command("xdotool", "click", "1")
	leftClickCmd.Env = append(os.Environ(), "DISPLAY=:44")
	leftClickCmd.Run()

	// rightClickCmd := exec.Command("xdotool", "mousedown", "--window", "$(xdotool search --class minecraft | head -1)", "3")
	// rightClickCmd.Env = append(os.Environ(), "DISPLAY=:44")
	// for {
	// 	// For future implementation of mouse control
	// 	time.Sleep(time.Second)
	// }
}

func main() {
	// Ensure that the terminal has been wiped
	fmt.Print("\033[H\033[2J")
	fmt.Println("Terminal Minecraft Viewer")
	fmt.Println("Loading Minecraft stream...")
	fmt.Println("Press any key to begin capturing input")
	
	// Wait for Minecraft to be ready
	time.Sleep(5 * time.Second)
	
	// Start the keyboard input capture
	go CaptureKeyboardInput()
	go SendKeyboardToMinecraft()
	
	// Start rendering the Minecraft stream directly
	go RenderMinecraftDirectly()
	
	// Keep the main thread running
	DisplayRenderThread()
}

func init() {
	go DisplayRenderThread()
}
