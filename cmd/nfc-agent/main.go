package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/SimplyPrint/nfc-agent/internal/api"
	"github.com/SimplyPrint/nfc-agent/internal/config"
	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/SimplyPrint/nfc-agent/internal/service"
	"github.com/SimplyPrint/nfc-agent/internal/settings"
	"github.com/SimplyPrint/nfc-agent/internal/tray"
	"github.com/SimplyPrint/nfc-agent/internal/welcome"
)

func main() {
	// Define flags
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	noTrayFlag := flag.Bool("no-tray", false, "Run without system tray (headless mode)")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "NFC Agent - Local NFC card reader service\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  nfc-agent [flags]\n")
		fmt.Fprintf(os.Stderr, "  nfc-agent <command>\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  install     Install auto-start service\n")
		fmt.Fprintf(os.Stderr, "  uninstall   Remove auto-start service\n")
		fmt.Fprintf(os.Stderr, "  version     Print version information\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  NFC_AGENT_PORT    Port to listen on (default: 32145)\n")
		fmt.Fprintf(os.Stderr, "  NFC_AGENT_HOST    Host to bind to (default: 127.0.0.1)\n")
	}

	flag.Parse()

	// Handle version flag
	if *versionFlag {
		printVersion()
		return
	}

	// Handle commands (non-flag arguments)
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "version":
			printVersion()
			return
		case "install":
			if err := installService(); err != nil {
				log.Fatalf("Failed to install service: %v", err)
			}
			fmt.Println("Auto-start service installed successfully")
			return
		case "uninstall":
			if err := uninstallService(); err != nil {
				log.Fatalf("Failed to uninstall service: %v", err)
			}
			fmt.Println("Auto-start service removed successfully")
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
			flag.Usage()
			os.Exit(1)
		}
	}

	// Load configuration
	cfg := config.Load()

	// Start the server
	run(cfg, *noTrayFlag)
}

func printVersion() {
	fmt.Printf("nfc-agent %s\n", api.Version)
	fmt.Printf("Build time: %s\n", api.BuildTime)
	fmt.Printf("Git commit: %s\n", api.GitCommit)
}

func run(cfg *config.Config, headless bool) {
	// Top-level panic recovery - log crash and exit gracefully
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()

			// Send to Sentry if enabled
			logging.CapturePanic(rec, stack, "main")

			// Try to log to crash file
			crashFile, err := logging.WriteCrashLog(rec, stack)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write crash log: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "Crash log written to: %s\n", crashFile)
			}

			// Print to stderr
			fmt.Fprintf(os.Stderr, "\n=== FATAL PANIC ===\n%v\n\nStack trace:\n%s\n", rec, string(stack))

			// Log to in-memory logger (may not persist but helps if we can recover)
			logging.Error(logging.CatSystem, fmt.Sprintf("FATAL PANIC: %v", rec), map[string]any{
				"panic": fmt.Sprintf("%v", rec),
				"stack": string(stack),
			})

			os.Exit(1)
		}
	}()

	// Initialize logging system
	logging.Init(1000, logging.LevelDebug)

	// Load user settings
	userSettings, _ := settings.Load()

	// Initialize Sentry for crash reporting (opt-in via settings or NFC_AGENT_SENTRY=1)
	if logging.InitSentry(api.Version, userSettings.CrashReporting) {
		defer logging.FlushSentry(2 * time.Second)
	}

	logging.Info(logging.CatSystem, "NFC Agent starting", map[string]any{
		"version": api.Version,
	})

	// Initialize update checker
	api.InitUpdateChecker()

	// Set up shutdown handler for API endpoint
	api.SetShutdownHandler(func() {
		log.Println("Shutting down...")
		os.Exit(0)
	})

	mux := api.NewMux()

	// Add WebSocket endpoint
	mux.HandleFunc("/v1/ws", api.InitWebSocket())

	addr := cfg.Address()

	// Server start function
	startServer := func() {
		log.Printf("nfc-agent %s listening on http://%s\n", api.Version, addr)
		log.Printf("WebSocket available at ws://%s/v1/ws\n", addr)
		logging.Info(logging.CatSystem, "Server started", map[string]any{
			"address": addr,
		})

		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}

	// Determine if we should use system tray
	useTray := !headless && tray.IsSupported()

	if useTray {
		log.Println("Starting with system tray...")

		// Show welcome popup and prompts on first run
		if welcome.IsFirstRun() {
			go func() {
				welcome.ShowWelcome()

				// Check if auto-start is not already configured (e.g., by Homebrew)
				svc := service.New()
				if !svc.IsInstalled() {
					// Prompt user to enable auto-start
					if welcome.PromptAutostart() {
						if err := svc.Install(); err != nil {
							log.Printf("Failed to enable auto-start: %v", err)
						} else {
							log.Println("Auto-start enabled")
						}
					}
				}

				// Prompt for crash reporting
				if welcome.PromptCrashReporting() {
					if err := settings.SetCrashReporting(true); err != nil {
						log.Printf("Failed to save crash reporting setting: %v", err)
					} else {
						log.Println("Crash reporting enabled")
					}
				}

				_ = welcome.MarkAsShown() // Ignore error - non-critical
			}()
		}

		// Create tray app with quit handler
		trayApp := tray.New(addr, func() {
			log.Println("Shutting down...")
			os.Exit(0)
		})

		// Run tray with server - this blocks on the main thread until quit
		// (required for macOS Cocoa compatibility)
		trayApp.RunWithServer(startServer)
	} else {
		if headless {
			log.Println("Running in headless mode (no system tray)")
		} else {
			log.Println("System tray not supported on this platform, running headless")
		}

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			log.Println("Shutting down...")
			os.Exit(0)
		}()

		startServer()
	}
}

// installService installs the auto-start service for the current platform.
func installService() error {
	svc := service.New()
	return svc.Install()
}

// uninstallService removes the auto-start service for the current platform.
func uninstallService() error {
	svc := service.New()
	return svc.Uninstall()
}
