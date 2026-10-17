package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/Ehsanamn1/alpha/internal/config"
	"github.com/Ehsanamn1/alpha/internal/engine"
	"github.com/Ehsanamn1/alpha/internal/localui"
	"github.com/Ehsanamn1/alpha/internal/orchestrator"
	"github.com/Ehsanamn1/alpha/internal/router"
)

const version = "0.3.0-alpha"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("Alpha %s\n", version)
	case "engines":
		listEngines()
	case "validate":
		validateCommand(os.Args[2:])
	case "doctor":
		doctorCommand(os.Args[2:])
	case "run":
		runCommand(os.Args[2:])
	case "ui":
		uiCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`Alpha — multi-engine local SOCKS5 router (early prototype)

Usage:
  alpha version
  alpha engines
  alpha validate --config config.json
  alpha doctor --config config.json
  alpha run --config config.json [--allow-process-launch]
  alpha ui --config config.json [--allow-process-launch] [--no-open]

Commands:
  engines    Show adapter types and integration status
  validate   Validate configuration without opening network connections
  doctor     Probe enabled local SOCKS5 engines against configured probe targets
  run        Start configured local engine processes (opt-in) and the local router
  ui         Start the router plus the responsive local Alpha control panel

Security:
  Process launching is disabled unless --allow-process-launch is explicitly passed.
  Only use executable paths and configuration files you personally trust.
`)
}

func listEngines() {
	fmt.Println("Alpha engine catalog (found on this system != configured or verified):")
	fmt.Println("\n  ENGINE       EXECUTABLE                  INTEGRATION MODE")
	fmt.Println("  Diban        python / project script     external SOCKS5 endpoint")
	fmt.Println("  sing-box     sing-box                    managed/external SOCKS5")
	fmt.Println("  Xray         xray                        managed/external SOCKS5")
	fmt.Println("  Tor          tor                         managed/external SOCKS5; bridges configured separately")
	fmt.Println("  Psiphon      psiphon / client app        external endpoint; build-specific control needed")
	fmt.Println("  iodine       iodine                      research only; DNS tunnel is not SOCKS5")
	fmt.Println("  TheFeed      thefeed-server/client       emergency data/messages, not a full tunnel")
	fmt.Println("  Spoof Tunnel spoof / spoof-linux-*       research only; not a SOCKS5 engine")
	fmt.Println("\n  DETECTED EXECUTABLES:")
	for _, candidate := range []struct{ name, exe string }{
		{"sing-box", "sing-box"}, {"Xray", "xray"}, {"Tor", "tor"},
		{"Psiphon CLI (if installed)", "psiphon"}, {"iodine", "iodine"},
		{"TheFeed client", "thefeed-client"}, {"TheFeed server", "thefeed-server"},
		{"Spoof Tunnel", "spoof"},
	} {
		if path, err := exec.LookPath(candidate.exe); err == nil {
			fmt.Printf("    [FOUND] %-24s %s\n", candidate.name, path)
		} else {
			fmt.Printf("    [----]  %-24s not found in PATH\n", candidate.name)
		}
	}
	fmt.Println("\nDiban, sing-box, Xray, Tor and some Psiphon clients can be used only when a local SOCKS5 endpoint is available.")
	fmt.Println("DNS tunnels, emergency feeds and Spoof Tunnel are deliberately not advertised as integrated until an adapter passes its own security and end-to-end tests.")
}

func loadConfig(path string) (config.Config, error) {
	if path == "" {
		path = "config.json"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(abs)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func validateCommand(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	path := fs.String("config", "config.json", "Path to configuration JSON")
	_ = fs.Parse(args)
	cfg, err := loadConfig(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "INVALID:", err)
		os.Exit(1)
	}
	fmt.Printf("Valid configuration: %s\n", *path)
	fmt.Printf("Router listen address: %s\n", cfg.Listen)
	fmt.Printf("Configured engine profiles: %d\n", len(cfg.Engines))
	fmt.Printf("Probe targets: %v\n", cfg.ProbeTargets())
}

func doctorCommand(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	path := fs.String("config", "config.json", "Path to configuration JSON")
	_ = fs.Parse(args)
	cfg, err := loadConfig(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CONFIG ERROR:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ProbeTimeout())
	defer cancel()
	status := orchestrator.ProbeProfiles(ctx, cfg)
	if len(status) == 0 {
		fmt.Println("No enabled engines. Enable at least one configured engine profile.")
		return
	}
	available := 0
	for _, s := range status {
		if s.Healthy {
			available++
			fmt.Printf("[OK]   %-18s %s  (%s)\n", s.Name, s.Address, s.Latency.Round(timePrecision()).String())
		} else {
			fmt.Printf("[DOWN] %-18s %s  (%s)\n", s.Name, s.Address, s.Error)
		}
	}
	fmt.Printf("\nHealthy end-to-end routes: %d/%d\n", available, len(status))
	if available == 0 {
		fmt.Println("No working route was verified. This does not distinguish filtering from server/configuration failure by itself.")
		os.Exit(1)
	}
}

func timePrecision() time.Duration { return 10 * time.Millisecond }

func runCommand(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	path := fs.String("config", "config.json", "Path to configuration JSON")
	allowLaunch := fs.Bool("allow-process-launch", false, "Allow configured local executables to be launched")
	_ = fs.Parse(args)
	cfg, err := loadConfig(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CONFIG ERROR:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	processes := startConfiguredProcesses(cfg, *allowLaunch)
	defer func() {
		for i := len(processes) - 1; i >= 0; i-- {
			_ = processes[i].Stop()
		}
	}()

	manager := orchestrator.New(cfg)
	go manager.Monitor(ctx)
	server := router.New(cfg.Listen, manager)
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(ctx) }()
	fmt.Printf("Router listening on %s\n", cfg.Listen)
	fmt.Printf("Probe targets: %v\n", cfg.ProbeTargets())
	fmt.Println("Configure applications to use this local SOCKS5 endpoint. Press Ctrl+C to stop.")
	select {
	case <-ctx.Done():
		fmt.Println("Stopping...")
	case err := <-errCh:
		if err != nil {
			fmt.Fprintln(os.Stderr, "Router stopped:", err)
			os.Exit(1)
		}
	}
}

func startConfiguredProcesses(cfg config.Config, allowLaunch bool) []*engine.Process {
	var processes []*engine.Process
	for _, spec := range cfg.Engines {
		if !spec.Enabled || !spec.StartProcess {
			continue
		}
		if !allowLaunch {
			fmt.Fprintf(os.Stderr, "Skipping process launch for %q: set --allow-process-launch after reviewing config.json.\n", spec.Name)
			continue
		}
		p, err := engine.Start(spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not start %s: %v\n", spec.Name, err)
			continue
		}
		processes = append(processes, p)
		fmt.Printf("Started local engine process: %s (pid %d)\n", spec.Name, p.PID())
		if !waitForListener(spec.SOCKSAddr, time.Duration(cfg.EngineStartTimeoutSecs)*time.Second) {
			fmt.Fprintf(os.Stderr, "Engine %q did not open %s within %s; monitoring will continue.\n", spec.Name, spec.SOCKSAddr, time.Duration(cfg.EngineStartTimeoutSecs)*time.Second)
		}
	}
	return processes
}

// uiCommand runs the local router and the loopback-only responsive dashboard.
// Process launch remains opt-in so unreviewed commands from config are never
// executed just because the UI was opened.
func uiCommand(args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	path := fs.String("config", "config.json", "Path to configuration JSON")
	listen := fs.String("listen", "127.0.0.1:18888", "Local UI address (loopback only)")
	allowLaunch := fs.Bool("allow-process-launch", false, "Allow configured local executables to be launched")
	noOpen := fs.Bool("no-open", false, "Do not open the control panel in a browser")
	_ = fs.Parse(args)
	cfg, err := loadConfig(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CONFIG ERROR:", err)
		os.Exit(1)
	}
	parent, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cfg = withImportedSingBoxRoute(cfg)
	processes := startConfiguredProcesses(cfg, *allowLaunch)
	defer func() {
		for i := len(processes) - 1; i >= 0; i-- {
			_ = processes[i].Stop()
		}
	}()
	manager := orchestrator.New(cfg)
	go manager.Monitor(ctx)
	routerServer := router.New(cfg.Listen, manager)
	routerErr := make(chan error, 1)
	go func() { routerErr <- routerServer.Serve(ctx) }()
	uiServer, err := localui.New(cfg, manager, *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "UI ERROR:", err)
		os.Exit(1)
	}
	url := "http://" + *listen + "/"
	fmt.Printf("Alpha local dashboard: %s\n", url)
	fmt.Printf("SOCKS5 router: %s\n", cfg.Listen)
	fmt.Println("Dashboard binds to loopback. Imported configs run through sing-box only after you explicitly press Connect.")
	if !*noOpen {
		openBrowser(url)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- uiServer.Serve(ctx, *listen) }()
	select {
	case <-parent.Done():
		fmt.Println("Stopping Alpha...")
	case err := <-serveErr:
		if err != nil {
			fmt.Fprintln(os.Stderr, "UI stopped:", err)
		}
		cancel()
	case err := <-routerErr:
		if err != nil {
			fmt.Fprintln(os.Stderr, "Router stopped:", err)
		}
		cancel()
	}
}

func withImportedSingBoxRoute(cfg config.Config) config.Config {
	for _, engine := range cfg.Engines {
		if engine.ID == "alpha-imported-sing-box" {
			return cfg
		}
	}
	cfg.Engines = append(cfg.Engines, config.EngineConfig{
		ID: "alpha-imported-sing-box", Name: "Alpha · imported sing-box nodes", Adapter: "external-socks5",
		Enabled: true, Priority: -10, SOCKSAddr: "127.0.0.1:2089",
	})
	return cfg
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Open %s manually in your browser.\n", url)
		return
	}
	go func() { _ = cmd.Wait() }()
}

func waitForListener(address string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
