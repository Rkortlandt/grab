package daemon

import (
	"encoding/gob"
	"fmt"
	"grab/comm"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"
)

const SocketPath = "/tmp/grab.sock"

var StartTime time.Time
var history = make([][]string, 0, 10)

func init() {
	gob.Register(comm.ClipboardRequest{})
	gob.Register(comm.HistoryRequest{})
	gob.Register(comm.StatusRequest{})
	gob.Register(comm.SwitchHistoryRequest{})
	gob.Register(comm.GrabRequest{})
}

func Start() error {
	executable, _ := os.Executable()
	cmd := exec.Command(executable, "--daemon-internal")
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Start()
	if err != nil {
		return err
	}

	for i := 0; i < 100; i++ {
		if _, err := os.Stat(SocketPath); err == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("daemon failed to initialize socket at %s", SocketPath)
}

func RunServer() {
	StartTime = time.Now()
	os.Remove(SocketPath)

	l, err := net.Listen("unix", SocketPath)
	if err != nil {
		fmt.Printf("Fatal: Could not listen on socket: %v\n", err)
		return
	}
	defer l.Close()

	var clipboardContent []string

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		handleRequest(conn, &clipboardContent)
	}
}

func handleRequest(conn net.Conn, clipboard *[]string) {
	defer conn.Close()

	decoder := gob.NewDecoder(conn)
	encoder := gob.NewEncoder(conn)

	var input any
	if err := decoder.Decode(&input); err != nil {
		// Output to stderr to help debug if the daemon is run interactively
		fmt.Fprintf(os.Stderr, "Daemon decode error: %v\n", err)
		return
	}

	switch v := input.(type) {
	case comm.ClipboardRequest:
		encoder.Encode(comm.ClipboardResponse{Files: *clipboard})

	case comm.HistoryRequest:
		encoder.Encode(comm.HistoryResponse{History: history})

	case comm.StatusRequest:
		uptime := time.Since(StartTime).Round(time.Second)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		statusMsg := fmt.Sprintf("Uptime: %s | Memory: %d KB | Items in Clip: %d",
			uptime, m.Alloc/1024, len(*clipboard))
		encoder.Encode(comm.ActionResponse{Message: statusMsg})

	case comm.SwitchHistoryRequest:
		if v.Index >= 0 && v.Index < len(history) {
			selected := history[v.Index]
			// Rotate history
			history = append(history[:v.Index], history[v.Index+1:]...)
			history = append([][]string{selected}, history...)
			*clipboard = selected

			encoder.Encode(comm.ActionResponse{Message: fmt.Sprintf("Switched to history index: [%d]", v.Index)})
		} else {
			encoder.Encode(comm.ActionResponse{Error: "Error: Index out of range"})
		}

	case comm.GrabRequest:
		if len(v.Files) > 0 {
			*clipboard = v.Files
			// Prepend to history
			history = append([][]string{v.Files}, history...)
			if len(history) > 10 {
				history = history[:10]
			}
			encoder.Encode(comm.ActionResponse{Message: "Success"})
		} else {
			encoder.Encode(comm.ActionResponse{Error: "Error: No files provided"})
		}
	default:
		encoder.Encode(comm.ActionResponse{Error: "Error: Unknown request type"})
	}
}
