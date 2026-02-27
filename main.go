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
)

func init() {
	gob.Register(comm.ClipboardRequest{})
	gob.Register(comm.HistoryRequest{})
	gob.Register(comm.StatusRequest{})
	gob.Register(comm.SwitchHistoryRequest{})
	gob.Register(comm.GrabRequest{})
}

// --- File Operations ---

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

// --- CLI Client Struct ---

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
	var resp comm.ClipboardResponse
	if err := c.decoder.Decode(&resp); err != nil {
		fmt.Printf("Error decoding daemon response: %v\n", err)
		return
	}
	if resp.Error != "" {
		fmt.Println(resp.Error)
		return
	}
	if len(resp.Files) == 0 {
		fmt.Println("Clipboard is empty!")
		return
	}

	for _, path := range resp.Files {
		destination := filepath.Join(c.wd, filepath.Base(path))
		copyFile(path, destination)
	}
	fmt.Printf("✓ Pasted %d file(s)!\n", len(resp.Files))
}

func (c *Client) moveFiles() {
	c.send(comm.ClipboardRequest{})
	var resp comm.ClipboardResponse
	if err := c.decoder.Decode(&resp); err != nil {
		fmt.Printf("Error decoding daemon response: %v\n", err)
		return
	}
	if resp.Error != "" {
		fmt.Println(resp.Error)
		return
	}
	if len(resp.Files) == 0 {
		fmt.Println("Clipboard is empty!")
		return
	}

	for _, path := range resp.Files {
		destination := filepath.Join(c.wd, filepath.Base(path))
		moveFile(path, destination)
	}
	fmt.Printf("✓ Moved %d file(s)!\n", len(resp.Files))
}

func (c *Client) listHistory() {
	c.send(comm.HistoryRequest{})
	var resp comm.HistoryResponse
	if err := c.decoder.Decode(&resp); err != nil {
		fmt.Printf("Error decoding history: %v\n", err)
		return
	}
	if resp.Error != "" {
		fmt.Println(resp.Error)
		return
	}

	home, _ := os.UserHomeDir()
	for index, group := range slices.Backward(resp.History) {
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
	var resp comm.ActionResponse
	if err := c.decoder.Decode(&resp); err != nil {
		fmt.Printf("Error decoding status: %v\n", err)
		return
	}
	fmt.Printf("Grab Daemon Status:\n%s\n", resp.Message)
}

func (c *Client) switchHistory(index int) {
	c.send(comm.SwitchHistoryRequest{Index: index})
	var resp comm.ActionResponse
	if err := c.decoder.Decode(&resp); err != nil {
		fmt.Printf("Error switching history: %v\n", err)
		return
	}
	if resp.Error != "" {
		fmt.Println(resp.Error)
	} else {
		fmt.Println(resp.Message)
	}
}

func (c *Client) performGrab(args []string) {
	var allFiles []string
	for _, arg := range args {
		// Expand globs (handles the * operator)
		matches, err := filepath.Glob(filepath.Join(c.wd, arg))
		if err == nil && len(matches) > 0 {
			allFiles = append(allFiles, matches...)
		} else {
			// Fallback for direct paths
			absPath := filepath.Join(c.wd, arg)
			if _, err := os.Stat(absPath); err == nil {
				allFiles = append(allFiles, absPath)
			}
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

func handleCLI(conn net.Conn) {
	if len(os.Args) < 2 {
		fmt.Println("Usage: grab [filenames... | paste | move | list | status]")
		return
	}

	client, err := NewClient(conn)
	if err != nil {
		fmt.Printf("Error initializing client: %v\n", err)
		return
	}

	command := os.Args[1]

	switch command {
	case "paste":
		client.pasteFiles()
	case "move":
		client.moveFiles()
	case "list":
		client.listHistory()
	case "status":
		client.showStatus()
	default:
		if index, err := strconv.Atoi(command); err == nil {
			client.switchHistory(index)
		} else {
			client.performGrab(os.Args[1:])
		}
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--daemon-internal" {
		daemon.RunServer()
		return
	}

	conn, err := net.Dial("unix", daemon.SocketPath)
	if err != nil {
		fmt.Println("Starting clipboard daemon...")
		if err := daemon.Start(); err != nil {
			fmt.Printf("Fatal Error: %v\n", err)
			return
		}
		// Small delay might be needed depending on OS scheduling, but Start() waits for socket
		conn, err = net.Dial("unix", daemon.SocketPath)
		if err != nil {
			fmt.Printf("Could not connect to newly started daemon: %v\n", err)
			return
		}
	}
	defer conn.Close()

	handleCLI(conn)
}
