package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const (
	pidFile = "/tmp/poche-resend-webmail.pid"
	logFile = "/tmp/poche-resend-webmail.log"
)

func startDaemon(port int) {
	if isDaemonRunning() {
		fmt.Println("Daemon is already running")
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(execPath, "serve", fmt.Sprintf("-port=%d", port))
	logFileHandle, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFileHandle.Close()

	cmd.Stdout = logFileHandle
	cmd.Stderr = logFileHandle

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing PID file: %v\n", err)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}

	fmt.Printf("Daemon started with PID %d\n", pid)
	fmt.Printf("Logs: %s\n", logFile)
}

func stopDaemon() {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Daemon is not running")
			return
		}
		fmt.Fprintf(os.Stderr, "Error reading PID file: %v\n", err)
		os.Exit(1)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid PID file: %v\n", err)
		os.Exit(1)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Process not found: %v\n", err)
		_ = os.Remove(pidFile)
		os.Exit(1)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping daemon: %v\n", err)
		os.Exit(1)
	}

	_ = os.Remove(pidFile)
	fmt.Printf("Daemon stopped (PID %d)\n", pid)
}

func checkDaemonStatus() {
	if !isDaemonRunning() {
		fmt.Println(`{"ok":true,"running":false}`)
		return
	}
	pidData, _ := os.ReadFile(pidFile)
	fmt.Printf(`{"ok":true,"running":true,"pid":%s}`+"\n", strings.TrimSpace(string(pidData)))
}

func isDaemonRunning() bool {
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
