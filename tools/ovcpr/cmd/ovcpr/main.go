package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/diffscope/ovcpr/internal/repository"
	"os"
	"os/signal"
	"time"
)

func run(ctx context.Context, args []string) (string, any, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("usage: ovcpr validate|structure|build|export|classify")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("repository", ".", "candidate repository data root")
	baseline := flags.String("baseline", "", "protected baseline data root")
	candidateSHA := flags.String("candidate-revision", "", "fixed candidate commit")
	baselineSHA := flags.String("baseline-revision", "", "fixed baseline commit")
	revision := flags.String("revision", "", "fixed source commit")
	output := flags.String("output", "", "new output directory")
	policyFile := flags.String("network-policy", "", "trusted network policy JSON")
	cache := flags.String("cache-dir", "", "temporary archive directory")
	offline := flags.Bool("offline", false, "structure, immutability and graph checks only")
	_ = flags.Bool("json", false, "emit JSON report (always used)")
	if err := flags.Parse(args[1:]); err != nil {
		return command, nil, err
	}
	if flags.NArg() != 0 {
		return command, nil, fmt.Errorf("unexpected positional arguments")
	}
	switch command {
	case "validate", "structure":
		p, err := repository.ReadPolicy(*policyFile)
		if err != nil {
			return command, nil, err
		}
		data, err := repository.Validate(ctx, repository.ValidationOptions{Repository: *root, Baseline: *baseline, CandidateRevision: *candidateSHA, BaselineRevision: *baselineSHA, CacheDir: *cache, Policy: p, Offline: *offline || command == "structure"})
		return command, data, err
	case "build":
		if *output == "" {
			return command, nil, fmt.Errorf("output required")
		}
		data, err := repository.Build(ctx, *root, *output, *revision)
		return command, data, err
	case "export":
		if *output == "" {
			return command, nil, fmt.Errorf("output required")
		}
		err := repository.Export(ctx, *root, *revision, *output)
		return command, map[string]string{"repositoryRevision": *revision, "output": *output}, err
	case "classify":
		data, err := repository.Classify(ctx, *root, *baselineSHA, *candidateSHA)
		return command, data, err
	case "version":
		return command, map[string]string{"toolVersion": repository.ToolVersion, "dspmVersion": repository.LibraryVersion()}, nil
	default:
		return command, nil, fmt.Errorf("unknown command")
	}
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 55*time.Minute)
	defer cancel()
	command, data, err := run(ctx, os.Args[1:])
	report := map[string]any{"protocolVersion": 1, "command": command, "ok": err == nil, "diagnostics": []string{}}
	if err == nil {
		report["data"] = data
	} else {
		var e *repository.Error
		if !errors.As(err, &e) {
			e = &repository.Error{Code: "INTERNAL_ERROR", Message: err.Error()}
		}
		report["error"] = e
		report["partialData"] = data
	}
	if e := json.NewEncoder(os.Stdout).Encode(report); e != nil {
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}
