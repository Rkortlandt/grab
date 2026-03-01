package main

import (
	"encoding/gob"
	"fmt"
	"grab/comm"
	"grab/daemon"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func init() {
	gob.Register(comm.ClipboardRequest{})
	gob.Register(comm.HistoryRequest{})
	gob.Register(comm.StatusRequest{})
	gob.Register(comm.SwitchHistoryRequest{})
	gob.Register(comm.GrabRequest{})
}

func copyFile(src string, dst string) {
	fmt.Printf("Copying from: %v to: %v\n", src, dst)
	cmd := exec.Command("cp", src, dst)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error: cp failed for %s. %v\n", src, err)
	}
}

func moveFile(src string, dst string) {
	fmt.Printf("Moving from: %v to: %v\n", src, dst)
	cmd := exec.Command("mv", src, dst)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error: mv failed for %s. %v\n", src, err)
	}
}

type Client struct {
	encoder *gob.Encoder
	decoder *gob.Decoder
	wd      string
}

func NewClient(conn net.Conn) (*Client, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &Client{
		encoder: gob.NewEncoder(conn),
		decoder: gob.NewDecoder(conn),
		wd:      wd,
	}, nil
}

// The Fix: Helper to ensure requests are encoded as interfaces
func (c *Client) send(req any) error {
	return c.encoder.Encode(&req)
}

// --- CLI Command Handlers ---

func (c *Client) pasteFiles() {
	c.send(comm.ClipboardRequest{})
	var response comm.ClipboardResponse

	if err := c.decoder.Decode(&response); err != nil {
		fmt.Printf("Error decoding daemon response: %v\n", err)
		return
	}
	if response.Error != "" {
		fmt.Println(response.Error)
		return
	}
	if len(response.Files) == 0 {
		fmt.Println("Clipboard is empty!")
		return
	}

	for _, path := range response.Files {
		destination := filepath.Join(c.wd, filepath.Base(path))
		copyFile(path, destination)
	}
	fmt.Printf("✓ Pasted %d file(s)!\n", len(response.Files))
}

func (c *Client) moveFiles() {
	c.send(comm.ClipboardRequest{})
	var response comm.ClipboardResponse
	if err := c.decoder.Decode(&response); err != nil {
		fmt.Printf("Error decoding daemon response: %v\n", err)
		return
	}
	if response.Error != "" {
		fmt.Println(response.Error)
		return
	}
	if len(response.Files) == 0 {
		fmt.Println("Clipboard is empty!")
		return
	}

	for _, path := range response.Files {
		destination := filepath.Join(c.wd, filepath.Base(path))
		moveFile(path, destination)
	}
	fmt.Printf("✓ Moved %d file(s)!\n", len(response.Files))
}

func (c *Client) listHistory() {
	c.send(comm.HistoryRequest{})
	var response comm.HistoryResponse
	if err := c.decoder.Decode(&response); err != nil {
		fmt.Printf("Error decoding history: %v\n", err)
		return
	}
	if response.Error != "" {
		fmt.Println(response.Error)
		return
	}

	home, _ := os.UserHomeDir()
	for index, group := range slices.Backward(response.History) {
		label := fmt.Sprintf("[%d]", index)
		if index == 0 {
			label = "[current]"
		}

		display := "Empty"
		if len(group) > 0 {
			first := strings.Replace(group[0], home, "~", 1)

			if len(group) > 1 {
				display = fmt.Sprintf("%s, (+ %d more)", first, len(group)-1)
			} else {
				display = first
			}
		}
		fmt.Printf("%s: %v\n", label, display)
	}
}

func (c *Client) showStatus() {
	c.send(comm.StatusRequest{})
	var response comm.ActionResponse

	if err := c.decoder.Decode(&response); err != nil {
		fmt.Printf("Error decoding status: %v\n", err)
		return
	}
	fmt.Printf("Grab Daemon Status:\n%s\n", response.Message)
}

func (c *Client) switchHistory(index int) {
	c.send(comm.SwitchHistoryRequest{Index: index})
	var response comm.ActionResponse

	if err := c.decoder.Decode(&response); err != nil {
		fmt.Printf("Error switching history: %v\n", err)
		return
	}
	if response.Error != "" {
		fmt.Println(response.Error)
	} else {
		fmt.Println(response.Message)
	}
}

func (c *Client) performGrab(args []string) {
	var allFiles []string
	for _, arg := range args {
		searchPath := filepath.Join(c.wd, arg)
		matches, err := filepath.Glob(searchPath)

		if err != nil {
			fmt.Printf("Warning: Malformed glob pattern '%s': %v\n", arg, err)
		}

		if len(matches) > 0 {
			fmt.Printf("Pattern '%s' expanded to %d file(s)\n", arg, len(matches))
			allFiles = append(allFiles, matches...)
			continue
		}

		info, err := os.Stat(searchPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("Error: File or pattern '%s' does not exist in directory %s\n", arg, c.wd)
			} else {
				fmt.Printf("Error: System error accessing '%s': %v\n", searchPath, err)
			}
		} else {
			fmt.Printf("Found literal path: %s (Size: %d bytes)\n", searchPath, info.Size())
			allFiles = append(allFiles, searchPath)
		}
	}

	if len(allFiles) > 0 {
		c.send(comm.GrabRequest{Files: allFiles})
		var resp comm.ActionResponse

		if err := c.decoder.Decode(&resp); err == nil && resp.Error == "" {
			fmt.Printf("✓ %d file(s) grabbed!\n", len(allFiles))
		} else {
			fmt.Printf("Error confirming grab with daemon: %v\n", err)
			if resp.Error != "" {
				fmt.Printf("Daemon reported: %s\n", resp.Error)
			}
		}
	} else {
		fmt.Println("Error: No valid files found.")
	}
}

func printHelp() {
	// ANSI Color Codes
	Yel := "\033[33m" // Yellow
	Cyn := "\033[36m" // Cyan
	Grn := "\033[32m" // Green
	Gra := "\033[90m" // Gray
	Res := "\033[0m"  // Reset

	fmt.Print(
		Yel + "Usage:" + Res + " grab [filenames...] | [history_number] | paste | move | list | status\n\n" +
			Cyn + "Commands " + Gra + "-------------------------------------------------------------------" + Res + "\n" +
			"  " + Grn + "[paste]" + Res + ": current selected history to current directory\n" +
			"  " + Grn + "[move]" + Res + " : moves current selected history to current directory\n" +
			"  " + Grn + "[list]" + Res + " : lists history\n" +
			"  " + Grn + "[status]" + Res + ": get status of daemon\n\n" +
			Cyn + "Inputs " + Gra + "---------------------------------------------------------------------" + Res + "\n" +
			"  " + Grn + "grab [filenames...]" + Res + ", grabs files into history for use\n" +
			"  " + Grn + "grab [history_number]" + Res + ", selects a value from history (e.g., grab 5)\n",
	)
}

func handleInput(connection net.Conn) {
	if len(os.Args) < 2 {
		fmt.Println("Usage: grab [filenames... | paste | move | list | status]")
		return
	}

	client, err := NewClient(connection)
	if err != nil {
		fmt.Printf("Error initializing client: %v\n", err)
		return
	}

	command := os.Args[1]

	switch command {
	case "paste":
		client.pasteFiles()
	case "pst":
		client.pasteFiles()
	case "move":
		client.moveFiles()
	case "mv":
		client.moveFiles()
	case "list":
		client.listHistory()
	case "hist":
		client.listHistory()
	case "history":
		client.listHistory()
	case "status":
		client.showStatus()
	case "help":
		printHelp()
	default:
		if index, err := strconv.Atoi(command); err == nil {
			client.switchHistory(index)
		} else {
			client.performGrab(os.Args[1:])
		}
	}
}

const SocketPath = "/tmp/grab.sock"

func startDaemon() (int, error) {
	executable, _ := os.Executable()
	cmd := exec.Command(executable, "--daemon-internal")
	cmd.Stdout = nil
	cmd.Stderr = nil

	failedToSpawnErr := cmd.Start()
	if failedToSpawnErr != nil {
		return 0, failedToSpawnErr
	}

	for i := 0; i < 100; i++ {
		if _, failedToFindSocketErr := os.Stat(SocketPath); failedToFindSocketErr == nil {
			return cmd.Process.Pid, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return 0, fmt.Errorf("Daemon failed to initialize socket at %s", SocketPath)
}

func main() {
	var isSelfDaemon bool = len(os.Args) > 1 && os.Args[1] == "--daemon-internal"
	if isSelfDaemon {
		daemon.RunServer(SocketPath)
		return
	}

	connection, connectionErr := net.Dial("unix", SocketPath)

	if connectionErr != nil {
		fmt.Println("Connection failed starting clipboard daemon...")
		pid, startErr := startDaemon()

		if startErr != nil {
			fmt.Printf("Fatal Error Creating Daemon: %v\n", startErr)
			return
		}

		fmt.Println("Connecting to daemon...")
		connection, connectionErr = net.Dial("unix", SocketPath)
		if connectionErr != nil {
			fmt.Printf("Could not connect to recently started daemon (PID %d). Attempting Cleaning up...\n", pid)

			p, err := os.FindProcess(pid)

			if err == nil {
				_ = p.Kill()
				_, _ = p.Wait()
			}
			return
		}
	}
	defer connection.Close()

	handleInput(connection)
}
