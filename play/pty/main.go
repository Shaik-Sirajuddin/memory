package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

var instructions = []string{
	"list files in current directory",
}

const (
	ctrlU = "\x15"

	// Bracketed paste mode.
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"

	// Common submit keys.
	enter = "\r"

	// Common Shift+Enter encodings.
	//
	// Many modern terminal apps use CSI-u:
	//   Shift+Enter = ESC [ 13 ; 2 u
	csiUShiftEnter = "\x1b[13;2u"

	// Some terminals/apps use modifyOtherKeys-style encoding:
	//   Shift+Enter = ESC [ 27 ; 2 ; 13 ~
	modifyOtherKeysShiftEnter = "\x1b[27;2;13~"
)

func submitSeq(name string) []byte {
	switch strings.ToLower(name) {
	case "enter", "cr":
		return []byte(enter)
	case "shift-enter", "shift_enter", "csi-u-shift-enter", "csi_u_shift_enter":
		return []byte(csiUShiftEnter)
	case "modify-other-keys-shift-enter", "modify_other_keys_shift_enter":
		return []byte(modifyOtherKeysShiftEnter)
	default:
		return []byte(enter)
	}
}

func pastePrompt(prompt string, submit []byte) []byte {
	payload := ctrlU + pasteStart + prompt + pasteEnd
	out := []byte(payload)
	out = append(out, submit...)
	return out
}

func main() {
	debug := flag.Bool("debug", false, "log raw stdin bytes to /tmp/proxy_stdin.log")
	delay := flag.Duration("delay", 5*time.Second, "delay before injecting prompt")
	submit := flag.String("submit", "shift-enter", "submit key: enter | shift-enter | modify-other-keys-shift-enter")
	flag.Parse()

	var dbg *os.File
	if *debug {
		var err error
		dbg, err = os.Create("/tmp/proxy_stdin.log")
		if err != nil {
			fmt.Fprintln(os.Stderr, "debug log:", err)
			os.Exit(1)
		}
		defer dbg.Close()
		fmt.Fprintln(os.Stderr, "debug: logging stdin bytes to /tmp/proxy_stdin.log")
	}

	codexPath, err := exec.LookPath("claude")
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex not found:", err)
		os.Exit(1)
	}

	cmd := exec.Command(codexPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptm, err := pty.Start(cmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pty start:", err)
		os.Exit(1)
	}
	defer ptm.Close()

	// Resize child PTY when parent terminal resizes.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			_ = pty.InheritSize(os.Stdin, ptm)
		}
	}()
	sigCh <- syscall.SIGWINCH

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "raw mode:", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	writeCh := make(chan []byte, 128)
	done := make(chan struct{})

	// Child output -> parent terminal.
	go func() {
		_, _ = io.Copy(os.Stdout, ptm)
		close(done)
	}()

	// Parent keyboard -> write queue.
	go func() {
		buf := make([]byte, 1024)

		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)

				if *debug && dbg != nil {
					fmt.Fprintf(dbg, "stdin bytes: %v\n", chunk)
					_ = dbg.Sync()
				}

				// Ctrl+C in raw mode.
				if n == 1 && chunk[0] == 3 {
					_ = term.Restore(int(os.Stdin.Fd()), oldState)
					_ = cmd.Process.Kill()
					os.Exit(0)
				}

				writeCh <- chunk
			}

			if err != nil {
				return
			}
		}
	}()

	// Auto inject after Codex has had time to render.
	if !*debug {
		go func() {
			time.Sleep(*delay)

			key := submitSeq(*submit)

			for _, instr := range instructions {
				writeCh <- pastePrompt(instr, key)
				time.Sleep(1200 * time.Millisecond)
			}
		}()
	}

	// Single writer to PTY master.
	go func() {
		for chunk := range writeCh {
			_, _ = ptm.Write(chunk)
		}
	}()

	<-done

	_ = term.Restore(int(os.Stdin.Fd()), oldState)
	_ = cmd.Wait()
}
