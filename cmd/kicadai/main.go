package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"kicadai/internal/config"
)

const usage = `kicadai is a Go client for KiCad's IPC API.

Usage:
  kicadai [global flags] <command>

Commands:
  config    Print resolved connection configuration
  help      Print this help text

Global flags:
  --socket string       KiCad IPC endpoint, for example ipc:///tmp/kicad/api.sock
  --token string        KiCad API token
  --client-name string  Client name sent to KiCad
  --timeout-ms int      IPC timeout in milliseconds
  --json                Print command output as JSON when supported
`

type cliOptions struct {
	socket     string
	token      string
	clientName string
	timeoutMS  int
	jsonOutput bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	opts, command, err := parse(args, stderr)
	if err != nil {
		return err
	}

	switch command {
	case "", "help":
		fmt.Fprint(stdout, usage)
		return nil
	case "config":
		return runConfig(opts, stdout)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, usage)
	}
}

func parse(args []string, stderr io.Writer) (cliOptions, string, error) {
	var opts cliOptions
	flags := flag.NewFlagSet("kicadai", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	flags.StringVar(&opts.socket, "socket", "", "KiCad IPC endpoint")
	flags.StringVar(&opts.token, "token", "", "KiCad API token")
	flags.StringVar(&opts.clientName, "client-name", "", "client name sent to KiCad")
	flags.IntVar(&opts.timeoutMS, "timeout-ms", 0, "IPC timeout in milliseconds")
	flags.BoolVar(&opts.jsonOutput, "json", false, "print JSON output when supported")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, "help", nil
		}

		return cliOptions{}, "", err
	}

	if flags.NArg() == 0 {
		return opts, "help", nil
	}

	return opts, flags.Arg(0), nil
}

func runConfig(opts cliOptions, stdout io.Writer) error {
	explicit := config.Explicit{
		SocketPath:  opts.socket,
		Token:       opts.token,
		ClientName:  opts.clientName,
		TimeoutMS:   opts.timeoutMS,
		Environment: os.Environ(),
	}

	resolved, err := config.Resolve(explicit)
	if err != nil {
		return err
	}

	if opts.jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(resolved.Redacted())
	}

	fmt.Fprintf(stdout, "socket_path: %s\n", resolved.SocketPath)
	fmt.Fprintf(stdout, "client_name: %s\n", resolved.ClientName)
	fmt.Fprintf(stdout, "timeout_ms: %d\n", resolved.TimeoutMS)
	fmt.Fprintf(stdout, "token: %s\n", resolved.Redacted().Token)
	return nil
}
