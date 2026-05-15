// Print Bridge — single-EXE installer.
//
// Embeds the agent, the tray, the PowerShell installer script and the
// docs via go:embed. On launch it:
//   1. Checks for administrator rights (UAC token elevation).
//   2. If not elevated, re-launches itself via ShellExecute("runas").
//   3. Extracts the embedded payload to a temp directory.
//   4. Runs the bundled install.ps1, streaming its output to the console.
//   5. Pauses for the user to read the result, then cleans up.
//
// No external installer toolchain (NSIS, Inno, WiX) is required — the
// installer is just another Go binary.
package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// utf8BOM is prepended to .ps1 files at extraction so PowerShell 5.1 reads
// them as UTF-8 instead of falling back to the legacy ANSI code page.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// setConsoleUTF8 switches the Windows console code page to 65001 (UTF-8)
// so accented French strings round-trip cleanly between our process and
// the PowerShell child. No-op on non-Windows builds.
func setConsoleUTF8() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	setOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	setInputCP := kernel32.NewProc("SetConsoleCP")
	_, _, _ = setOutputCP.Call(uintptr(65001))
	_, _, _ = setInputCP.Call(uintptr(65001))
}

//go:embed all:payload
var payload embed.FS

const banner = `
=================================================
   Print Bridge — installeur Windows
=================================================
`

func main() {
	// Make sure the console can display UTF-8 — PowerShell streams its
	// French output as UTF-8 and the default Windows console code page
	// (CP-850 / 1252) garbles it. setConsoleUTF8 also flips stdout so
	// our own writes use UTF-8 consistently.
	setConsoleUTF8()

	// We always want a visible console window; bail out fast if launched
	// in an unexpected way.
	defer pause("Appuie sur Entrée pour fermer cette fenêtre…")

	fmt.Print(banner)

	elevated, err := isElevated()
	if err != nil {
		fatalf("Impossible de vérifier les droits administrateur : %v", err)
	}
	if !elevated {
		fmt.Println("Cet installeur a besoin des droits administrateur.")
		fmt.Println("Une fenêtre UAC va apparaître…")
		time.Sleep(800 * time.Millisecond)
		if err := relaunchElevated(); err != nil {
			fatalf("L'élévation a échoué : %v", err)
		}
		// The elevated copy will run independently; this process exits.
		return
	}

	fmt.Println("Droits administrateur OK.")
	fmt.Println()

	tmp, err := extractPayload()
	if err != nil {
		fatalf("Extraction du payload : %v", err)
	}
	defer os.RemoveAll(tmp)

	fmt.Printf("Fichiers extraits dans %s\n\n", tmp)

	if err := runInstaller(tmp); err != nil {
		fatalf("Installation : %v", err)
	}

	fmt.Println()
	fmt.Println("=================================================")
	fmt.Println("  Installation terminée.")
	fmt.Println("=================================================")
}

// ----- Elevation ---------------------------------------------------------

// isElevated reports whether the current process token has the elevated
// flag (i.e. the user accepted a UAC prompt or the process inherits admin
// rights).
func isElevated() (bool, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false, err
	}
	defer token.Close()

	var elevation uint32
	var size uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)),
		&size,
	)
	if err != nil {
		return false, err
	}
	return elevation != 0, nil
}

// relaunchElevated restarts this binary via ShellExecute with the "runas"
// verb, which triggers a UAC prompt. The new process inherits the same
// command-line arguments. The current process returns immediately —
// callers should exit.
func relaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb := windows.StringToUTF16Ptr("runas")
	file := windows.StringToUTF16Ptr(exe)
	cwd, _ := os.Getwd()
	dir := windows.StringToUTF16Ptr(cwd)

	// SW_SHOWNORMAL = 1
	return windows.ShellExecute(0, verb, file, nil, dir, 1)
}

// ----- Payload extraction ------------------------------------------------

func extractPayload() (string, error) {
	tmpRoot := filepath.Join(os.TempDir(), fmt.Sprintf("print-bridge-setup-%d", os.Getpid()))
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return "", err
	}

	err := fs.WalkDir(payload, "payload", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "payload")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == ".gitkeep" {
			return nil
		}
		dst := filepath.Join(tmpRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := payload.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(strings.ToLower(rel), ".ps1") && !bytes.HasPrefix(data, utf8BOM) {
			data = append(utf8BOM, data...)
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	return tmpRoot, nil
}

// ----- Installer execution -----------------------------------------------

func runInstaller(workdir string) error {
	ps1 := filepath.Join(workdir, "install.ps1")
	if _, err := os.Stat(ps1); err != nil {
		return fmt.Errorf("install.ps1 absent du payload : %w", err)
	}
	// Force the PowerShell child process to emit UTF-8 on its output streams.
	// Without this, PowerShell 5.1 defaults to whatever [Console]::OutputEncoding
	// happens to be (usually OEM 437/850) and our pipe receives mojibake.
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		"[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); "+
			"[Console]::InputEncoding = [System.Text.UTF8Encoding]::new(); "+
			"& '"+ps1+"'",
	)
	cmd.Dir = workdir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go streamLines(stdout, os.Stdout)
	go streamLines(stderr, os.Stderr)
	return cmd.Wait()
}

func streamLines(src io.Reader, dst io.Writer) {
	s := bufio.NewScanner(src)
	for s.Scan() {
		fmt.Fprintln(dst, s.Text())
	}
}

// ----- Helpers -----------------------------------------------------------

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\nERREUR : "+format+"\n", args...)
	pause("Appuie sur Entrée pour fermer…")
	os.Exit(1)
}

func pause(msg string) {
	fmt.Print(msg)
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
