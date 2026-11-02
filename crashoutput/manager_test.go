package crashoutput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrashOutputProcesses(t *testing.T) {
	if os.Getenv("MOO_CRASH_MODE") != "" {
		t.Skip("parent only")
	}
	for _, mode := range []string{"panic", "closed", "recovered", "exit"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			manager, err := New(Config{Root: root, MaskKeys: []string{"token"}})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			before, err := manager.List(context.Background())
			if err != nil || len(before) != 0 {
				t.Fatal("New installed output", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashOutputProcess$")
			cmd.Env = []string{"HOME=" + os.Getenv("HOME"), "PATH=" + os.Getenv("PATH"), "MOO_CRASH_MODE=" + mode, "MOO_CRASH_ROOT=" + root, "GOTRACEBACK=single"}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()
			if mode == "recovered" {
				if err != nil {
					t.Fatal("recovered business process failed")
				}
			} else {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() == 0 {
					t.Fatal("expected genuine process exit")
				}
			}
			reports, err := manager.List(context.Background())
			if err != nil || len(reports) != 1 {
				t.Fatal("spool count", err)
			}
			report, err := manager.Inspect(context.Background(), reports[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if !report.MayBePartial || len(report.Digest) != 64 {
				t.Fatal("false complete report")
			}
			if mode == "panic" {
				if !report.OutputPresent || !bytes.Contains(stderr.Bytes(), []byte("fixed-crash-private")) || strings.Contains(report.MaskedText, "fixed-crash-private") || !strings.Contains(report.MaskedText, "token=***") {
					t.Fatal("original stderr or masked diagnostic missing")
				}
				raw, err := os.ReadFile(filepath.Join(root, "crash-"+report.ID+".log"))
				if err != nil || !bytes.Contains(raw, []byte("fixed-crash-private")) {
					t.Fatal("private raw evidence missing", err)
				}
			} else if report.OutputPresent || report.MaskedText != "" {
				t.Fatal("unexpected crash report for non-output case")
			}
		})
	}
}
func TestCrashOutputProcess(t *testing.T) {
	mode := os.Getenv("MOO_CRASH_MODE")
	if mode == "" {
		t.Skip("isolated child only")
	}
	manager, err := New(Config{Root: os.Getenv("MOO_CRASH_ROOT"), MaskKeys: []string{"token"}})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := manager.Install()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Install(); !errors.Is(err, ErrBusy) {
		t.Fatal("multiple active owners")
	}
	switch mode {
	case "panic":
		go func() { panic("token=fixed-crash-private") }()
		select {}
	case "closed":
		if err = owner.Close(); err != nil {
			t.Fatal(err)
		}
		if err = owner.Close(); err != nil {
			t.Fatal(err)
		}
		panic("token=fixed-crash-private")
	case "recovered":
		func() { defer func() { recover() }(); panic("token=fixed-crash-private") }()
		if err = manager.Close(); err != nil {
			t.Fatal(err)
		}
		if err = manager.Close(); err != nil {
			t.Fatal(err)
		}
	case "exit":
		os.Exit(7)
	default:
		t.Fatal("invalid child fixture")
	}
}
