package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/taigatappuri/AHC-Plaza/internal/config"
	"github.com/taigatappuri/AHC-Plaza/internal/process"
	"github.com/taigatappuri/AHC-Plaza/internal/store"
	"github.com/taigatappuri/AHC-Plaza/internal/tuning"
	bundled "github.com/taigatappuri/AHC-Plaza/internal/tuning/runtime"
)

func executeTune(args []string) error {
	action := "start"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		action = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("tune", flag.ContinueOnError)
	cfgPath := f.String("config", "ahc-plaza.toml", "project configuration")
	solver := f.String("solver", "", "single C++ source")
	input := f.String("input-dir", "", "input set")
	trials := f.Int("trials", 100, "additional candidate count")
	threads := f.Int("threads", 0, "case workers (0: automatic)")
	timeout := f.Int("timeout-ms", 0, "case timeout")
	seconds := f.Int("seconds", 0, "total active-time budget (0: unlimited)")
	seed := f.Int("seed", 42, "sampler seed")
	id := f.String("study", "", "saved Study ID")
	additional := f.Int("additional-trials", 0, "increase saved trial budget")
	_ = f.Bool("best", true, "export best candidate including baseline")
	if e := f.Parse(args); e != nil {
		return e
	}
	cfg, e := config.Load(*cfgPath)
	if e != nil {
		return e
	}
	unlock, e := process.LockProject(cfg.ProjectRoot)
	if e != nil {
		return e
	}
	defer unlock()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if action == "setup" {
		info, e := bundled.Setup(ctx, cfg.ProjectRoot)
		if e != nil {
			return e
		}
		if e = tuning.ToolHealth(ctx, filepath.Join(info.Path, "bin", "python3.12")); e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(info)
	}
	if action == "clean-runtime" {
		return bundled.Clean(cfg.ProjectRoot)
	}
	database, e := store.OpenSQLite(filepath.Join(cfg.PathRoot, "ahc-plaza.db"))
	if e != nil {
		return e
	}
	defer database.Close()
	if _, e = database.MarkUnfinishedRunsFailed(ctx, time.Now().UTC()); e != nil {
		return e
	}
	m, e := tuning.NewManager(cfg.ProjectRoot, cfg.FilePath, version, database)
	if e != nil {
		return e
	}
	defer m.Close()
	switch action {
	case "export":
		v, e := m.Export(ctx, *id, nil)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "start":
		scan, e := m.Scan(*solver)
		if e != nil {
			return e
		}
		parameters := scan.Parameters
		if profile, e := database.Profile(ctx, scan.Hash); e != nil {
			return e
		} else if len(profile) > 0 {
			if e = json.Unmarshal(profile, &parameters); e != nil {
				return e
			}
		}
		s, e := m.Start(ctx, tuning.StartRequest{Solver: *solver, InputDir: *input, SourceHash: scan.Hash, Parameters: parameters, Trials: *trials, Threads: *threads, TimeoutMS: *timeout, Seconds: *seconds, Seed: *seed})
		if e != nil {
			return e
		}
		*id = s.ID
	case "resume":
		s, e := m.Resume(ctx, *id, *additional)
		if e != nil {
			return e
		}
		*id = s.ID
	case "validate":
		_, e := m.Validate(ctx, *id, tuning.ValidationRequest{InputDir: *input, Threads: *threads})
		if e != nil {
			return e
		}
	default:
		return fmt.Errorf("unknown tune operation: %s", action)
	}
	fmt.Fprintln(os.Stderr, "Study:", *id)
	if e = m.Wait(ctx, *id); e != nil {
		m.BeginShutdown()
		m.Close()
	}
	s, e := database.GetStudy(context.Background(), *id)
	if e != nil {
		return e
	}
	if e = json.NewEncoder(os.Stdout).Encode(s); e != nil {
		return e
	}
	if s.Status == "failed" {
		return fmt.Errorf("%s", s.Error)
	}
	return nil
}
