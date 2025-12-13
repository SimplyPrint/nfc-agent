package api

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/SimplyPrint/nfc-agent/internal/core"
	"github.com/SimplyPrint/nfc-agent/internal/data"
	"github.com/SimplyPrint/nfc-agent/internal/logging"
	"github.com/SimplyPrint/nfc-agent/internal/openprinttag"
	"github.com/SimplyPrint/nfc-agent/internal/service"
	"github.com/SimplyPrint/nfc-agent/internal/settings"
	"github.com/SimplyPrint/nfc-agent/internal/updater"
	"github.com/SimplyPrint/nfc-agent/internal/web"
)

// Version information (set via ldflags in production builds)
var (
	Version   = ""
	BuildTime = ""
	GitCommit = ""
)

func init() {
	// If version wasn't set via ldflags, this is a dev build
	// Try to get VCS info from Go's build info
	if Version == "" {
		Version = "dev"
		if info, ok := debug.ReadBuildInfo(); ok {
			var vcsRevision, vcsTime string
			var vcsModified bool
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					vcsRevision = setting.Value
				case "vcs.time":
					vcsTime = setting.Value
				case "vcs.modified":
					vcsModified = setting.Value == "true"
				}
			}
			if vcsRevision != "" {
				shortCommit := vcsRevision
				if len(shortCommit) > 7 {
					shortCommit = shortCommit[:7]
				}
				GitCommit = vcsRevision
				Version = "dev-" + shortCommit
				if vcsModified {
					Version += "-dirty"
				}
			}
			if vcsTime != "" {
				BuildTime = vcsTime
			}
		}
	}
}

// shutdownHandler is called when a shutdown is requested via API
var shutdownHandler func()

// updateChecker handles checking for updates from GitHub
var updateChecker *updater.Checker

// SetShutdownHandler sets the callback for shutdown requests
func SetShutdownHandler(handler func()) {
	shutdownHandler = handler
}

// InitUpdateChecker initializes the update checker with the current version
func InitUpdateChecker() {
	updateChecker = updater.NewChecker(Version)
}

// NewMux constructs and returns the HTTP mux for the API.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Serve embedded status web UI at root
	mux.Handle("/", web.Handler())

	// API routes
	mux.HandleFunc("/v1/readers", corsMiddleware(handleListReaders))
	mux.HandleFunc("/v1/readers/", corsMiddleware(handleReaderRoutes)) // Note the trailing slash for sub-paths
	mux.HandleFunc("/v1/supported-readers", corsMiddleware(handleSupportedReaders))
	mux.HandleFunc("/v1/version", corsMiddleware(handleVersion))
	mux.HandleFunc("/v1/health", corsMiddleware(handleHealth))
	mux.HandleFunc("/v1/logs", corsMiddleware(handleLogs))
	mux.HandleFunc("/v1/crashes", corsMiddleware(handleCrashes))
	mux.HandleFunc("/v1/settings", corsMiddleware(handleSettings))
	mux.HandleFunc("/v1/shutdown", corsMiddleware(handleShutdown))
	mux.HandleFunc("/v1/autostart", corsMiddleware(handleAutostart))
	mux.HandleFunc("/v1/updates", corsMiddleware(handleUpdates))
	return mux
}

// recoveryMiddleware catches panics and logs them to crash files.
func recoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				context := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)

				// Send to Sentry if enabled
				logging.CapturePanic(rec, stack, context)

				// Log to in-memory logger
				logging.Error(logging.CatHTTP, fmt.Sprintf("PANIC in %s: %v", context, rec), map[string]any{
					"panic":  fmt.Sprintf("%v", rec),
					"stack":  string(stack),
					"method": r.Method,
					"path":   r.URL.Path,
				})

				// Write crash log to file
				crashFile, err := logging.WriteCrashLog(rec, stack)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to write crash log: %v\n", err)
					crashFile = ""
				}

				// Print to stderr
				fmt.Fprintf(os.Stderr, "\n=== PANIC in %s ===\n%v\n\nStack trace:\n%s\n", context, rec, string(stack))

				// Send 500 response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":     "internal server error",
					"crashFile": crashFile,
				})
			}
		}()
		next(w, r)
	}
}

// corsMiddleware adds CORS headers to allow browser access from any origin.
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Wrap with recovery middleware
		recoveryMiddleware(next)(w, r)
	}
}

func handleListReaders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	readers := core.ListReaders()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(readers); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

func handleReaderRoutes(w http.ResponseWriter, r *http.Request) {
	// Parse path: /v1/readers/{index}/...
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid path",
		})
		return
	}

	readerIndex, err := strconv.Atoi(parts[2])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid reader index",
		})
		return
	}

	// Get all readers
	readers := core.ListReaders()
	if len(readers) == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "no readers found",
		})
		return
	}

	if readerIndex < 0 || readerIndex >= len(readers) {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "reader index out of range",
		})
		return
	}

	readerName := readers[readerIndex].Name

	// Route to appropriate handler based on path
	if len(parts) >= 4 {
		switch parts[3] {
		case "card":
			handleReaderCard(w, r, readerName)
		case "erase":
			handleEraseCard(w, r, readerName)
		case "lock":
			handleLockCard(w, r, readerName)
		case "password":
			handlePassword(w, r, readerName)
		case "records":
			handleMultipleRecords(w, r, readerName)
		case "mifare":
			handleMifareBlock(w, r, readerName, parts)
		case "ultralight":
			handleUltralightPage(w, r, readerName, parts)
		default:
			respondJSON(w, http.StatusNotFound, map[string]string{
				"error": "unknown endpoint",
			})
		}
	} else {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing endpoint (e.g., /card, /erase, /lock)",
		})
	}
}

func handleReaderCard(w http.ResponseWriter, r *http.Request, readerName string) {
	switch r.Method {
	case http.MethodGet:
		// Read card UID and info
		card, err := core.GetCardUID(readerName)
		if err != nil {
			logging.Debug(logging.CatHTTP, "Card read failed", map[string]any{
				"reader": readerName,
				"error":  err.Error(),
			})
			respondJSON(w, http.StatusNotFound, map[string]string{
				"error": err.Error(),
			})
			return
		}
		logData := map[string]any{
			"reader": readerName,
			"uid":    card.UID,
			"type":   card.Type,
		}
		if card.Data != "" {
			logData["data"] = card.Data
			logData["dataType"] = card.DataType
		}
		if card.URL != "" {
			logData["url"] = card.URL
		}
		logging.Info(logging.CatCard, "Tag read", logData)
		respondJSON(w, http.StatusOK, card)

	case http.MethodPost:
		// Write data to card
		var req struct {
			Data     string `json:"data"`     // Data to write (string for text/json, base64 for binary)
			DataType string `json:"dataType"` // "text", "json", "binary", or "url"
			URL      string `json:"url"`      // Optional URL to write as first record
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		// Validate data type
		if req.DataType == "" {
			req.DataType = "text" // Default to text
		}

		// Convert data based on type
		var dataBytes []byte
		switch req.DataType {
		case "text", "json", "url":
			dataBytes = []byte(req.Data)
		case "binary":
			// Decode base64 for binary data
			var err error
			dataBytes, err = base64.StdEncoding.DecodeString(req.Data)
			if err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{
					"error": "invalid base64 data for binary type",
				})
				return
			}
		case "openprinttag":
			// Validate JSON structure for openprinttag
			var input openprinttag.Input
			if err := json.Unmarshal([]byte(req.Data), &input); err != nil {
				respondJSON(w, http.StatusBadRequest, map[string]string{
					"error": "invalid openprinttag JSON format: " + err.Error(),
				})
				return
			}
			dataBytes = []byte(req.Data)
		default:
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "dataType must be 'text', 'json', 'binary', 'url', or 'openprinttag'",
			})
			return
		}

		// Write data to card (with optional URL)
		if err := core.WriteDataWithURL(readerName, dataBytes, req.DataType, req.URL); err != nil {
			logging.Error(logging.CatCard, "Tag write failed", map[string]any{
				"reader": readerName,
				"error":  err.Error(),
			})
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		logData := map[string]any{
			"reader":   readerName,
			"dataType": req.DataType,
			"dataLen":  len(dataBytes),
		}
		if req.URL != "" {
			logData["url"] = req.URL
		}
		logging.Info(logging.CatCard, "Tag written", logData)
		respondJSON(w, http.StatusOK, map[string]string{
			"success": "data written successfully",
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleEraseCard(w http.ResponseWriter, r *http.Request, readerName string) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	logging.Info(logging.CatCard, "Erasing card", map[string]any{
		"reader": readerName,
	})
	if err := core.EraseCard(readerName); err != nil {
		logging.Error(logging.CatCard, "Card erase failed", map[string]any{
			"reader": readerName,
			"error":  err.Error(),
		})
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	logging.Info(logging.CatCard, "Card erased successfully", map[string]any{
		"reader": readerName,
	})
	respondJSON(w, http.StatusOK, map[string]string{
		"success": "card erased successfully",
	})
}

func handleLockCard(w http.ResponseWriter, r *http.Request, readerName string) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Require confirmation parameter to prevent accidental locking
	var req struct {
		Confirm bool `json:"confirm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if !req.Confirm {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "must set confirm=true to lock card (WARNING: this is IRREVERSIBLE)",
		})
		return
	}

	logging.Warn(logging.CatCard, "Locking card permanently", map[string]any{
		"reader": readerName,
	})
	if err := core.LockCard(readerName); err != nil {
		logging.Error(logging.CatCard, "Card lock failed", map[string]any{
			"reader": readerName,
			"error":  err.Error(),
		})
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	logging.Warn(logging.CatCard, "Card locked permanently", map[string]any{
		"reader": readerName,
	})
	respondJSON(w, http.StatusOK, map[string]string{
		"success": "card locked permanently",
	})
}

func handlePassword(w http.ResponseWriter, r *http.Request, readerName string) {
	switch r.Method {
	case http.MethodPost:
		// Set password
		var req struct {
			Password  string `json:"password"`  // 4 bytes as hex string (8 chars)
			Pack      string `json:"pack"`      // 2 bytes as hex string (4 chars)
			StartPage int    `json:"startPage"` // Page number from which password protection starts
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		password, err := hex.DecodeString(req.Password)
		if err != nil || len(password) != 4 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "password must be 8 hex characters (4 bytes)",
			})
			return
		}

		pack, err := hex.DecodeString(req.Pack)
		if err != nil || len(pack) != 2 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "pack must be 4 hex characters (2 bytes)",
			})
			return
		}

		if req.StartPage < 4 {
			req.StartPage = 4 // Default to protecting from page 4 onwards
		}

		if err := core.SetPassword(readerName, password, pack, byte(req.StartPage)); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{
			"success": "password set successfully",
		})

	case http.MethodDelete:
		// Remove password
		var req struct {
			Password string `json:"password"` // Current password to authenticate
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		password, err := hex.DecodeString(req.Password)
		if err != nil || len(password) != 4 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "password must be 8 hex characters (4 bytes)",
			})
			return
		}

		if err := core.RemovePassword(readerName, password); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{
			"success": "password removed successfully",
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleMultipleRecords(w http.ResponseWriter, r *http.Request, readerName string) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Records []core.NDEFRecord `json:"records"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if len(req.Records) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "records array cannot be empty",
		})
		return
	}

	if err := core.WriteMultipleRecords(readerName, req.Records); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"success": "records written successfully",
	})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"version":   Version,
		"buildTime": BuildTime,
		"gitCommit": GitCommit,
	}

	// Include update info if available (for JS SDK / SimplyPrint integration)
	if updateChecker != nil {
		info := updateChecker.Check(false) // Use cached result
		response["updateAvailable"] = info.Available
		if info.LatestVersion != "" {
			response["latestVersion"] = info.LatestVersion
		}
		if info.ReleaseURL != "" {
			response["releaseUrl"] = info.ReleaseURL
		}
	}

	respondJSON(w, http.StatusOK, response)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Check if we can list readers (basic health check)
	readers := core.ListReaders()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"readerCount": len(readers),
	})
}

func handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	if shutdownHandler == nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "shutdown not available",
		})
		return
	}

	logging.Info(logging.CatSystem, "Shutdown requested via API", nil)
	respondJSON(w, http.StatusOK, map[string]string{
		"success": "shutting down",
	})

	// Trigger shutdown after response is sent
	go func() {
		shutdownHandler()
	}()
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) // Error logged but not returned (header already sent)
}

func handleSupportedReaders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	readers, err := data.GetSupportedReaders()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load supported readers",
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"readers": readers,
	})
}

func handleAutostart(w http.ResponseWriter, r *http.Request) {
	svc := service.New()

	switch r.Method {
	case http.MethodGet:
		// Get auto-start status
		installed := svc.IsInstalled()
		status, _ := svc.Status()

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"enabled": installed,
			"status":  status,
		})

	case http.MethodPost:
		// Enable auto-start
		if svc.IsInstalled() {
			respondJSON(w, http.StatusOK, map[string]string{
				"success": "auto-start already enabled",
			})
			return
		}

		if err := svc.Install(); err != nil {
			logging.Error(logging.CatSystem, "Failed to enable auto-start", map[string]any{
				"error": err.Error(),
			})
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		logging.Info(logging.CatSystem, "Auto-start enabled via API", nil)
		respondJSON(w, http.StatusOK, map[string]string{
			"success": "auto-start enabled",
		})

	case http.MethodDelete:
		// Disable auto-start
		if !svc.IsInstalled() {
			respondJSON(w, http.StatusOK, map[string]string{
				"success": "auto-start already disabled",
			})
			return
		}

		if err := svc.Uninstall(); err != nil {
			logging.Error(logging.CatSystem, "Failed to disable auto-start", map[string]any{
				"error": err.Error(),
			})
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		logging.Info(logging.CatSystem, "Auto-start disabled via API", nil)
		respondJSON(w, http.StatusOK, map[string]string{
			"success": "auto-start disabled",
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Parse query parameters
		query := r.URL.Query()

		// Limit (default 100, max 1000)
		limit := 100
		if limitStr := query.Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
				if limit > 1000 {
					limit = 1000
				}
			}
		}

		// Min level filter
		var minLevel *logging.Level
		if levelStr := query.Get("level"); levelStr != "" {
			switch strings.ToLower(levelStr) {
			case "debug":
				l := logging.LevelDebug
				minLevel = &l
			case "info":
				l := logging.LevelInfo
				minLevel = &l
			case "warn":
				l := logging.LevelWarn
				minLevel = &l
			case "error":
				l := logging.LevelError
				minLevel = &l
			}
		}

		// Category filter
		var category *logging.Category
		if catStr := query.Get("category"); catStr != "" {
			c := logging.Category(catStr)
			category = &c
		}

		entries := logging.Get().GetEntries(limit, minLevel, category)
		stats := logging.Get().Stats()

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"entries": entries,
			"stats":   stats,
		})

	case http.MethodDelete:
		// Clear all logs
		logging.Get().Clear()
		respondJSON(w, http.StatusOK, map[string]string{
			"success": "logs cleared",
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleCrashes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		query := r.URL.Query()

		// Check if requesting a specific crash log
		filename := query.Get("file")
		if filename != "" {
			content, err := logging.ReadCrashLog(filename)
			if err != nil {
				respondJSON(w, http.StatusNotFound, map[string]string{
					"error": "crash log not found: " + err.Error(),
				})
				return
			}
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"filename": filename,
				"content":  content,
			})
			return
		}

		// List crash logs
		limit := 20
		if limitStr := query.Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
				if limit > 100 {
					limit = 100
				}
			}
		}

		logs, err := logging.GetCrashLogs(limit)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "failed to list crash logs: " + err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"crashes":  logs,
			"crashDir": logging.CrashLogDir(),
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleSettings handles GET and POST requests for user settings.
func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s := settings.Get()
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"crashReporting": s.CrashReporting,
		})

	case http.MethodPost:
		var req struct {
			CrashReporting *bool `json:"crashReporting"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body: " + err.Error(),
			})
			return
		}

		if req.CrashReporting != nil {
			if err := settings.SetCrashReporting(*req.CrashReporting); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "failed to save settings: " + err.Error(),
				})
				return
			}
		}

		s := settings.Get()
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"crashReporting": s.CrashReporting,
			"message":        "Settings updated. Restart may be required for some changes to take effect.",
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleUpdates checks for available updates from GitHub releases
func handleUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	// Initialize update checker if not already done
	if updateChecker == nil {
		InitUpdateChecker()
	}

	// Check if force refresh is requested
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	info := updateChecker.Check(forceRefresh)

	respondJSON(w, http.StatusOK, info)
}

// parseMifareKey parses a hex string into a 6-byte MIFARE key.
// Returns nil if the input is empty. Returns an error if the key is invalid.
func parseMifareKey(keyHex string) ([]byte, error) {
	if keyHex == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 6 {
		return nil, fmt.Errorf("invalid key (must be 12 hex characters)")
	}
	return key, nil
}

// parseMifareKeyType converts a key type string ("A" or "B") to a byte.
// Returns 'A' by default.
func parseMifareKeyType(kt string) byte {
	if kt == "B" || kt == "b" {
		return 'B'
	}
	return 'A'
}

// handleMifareBlock handles read/write operations on MIFARE Classic blocks
// GET /v1/readers/{n}/mifare/{block} - Read block
// POST /v1/readers/{n}/mifare/{block} - Write block
func handleMifareBlock(w http.ResponseWriter, r *http.Request, readerName string, parts []string) {
	// Expect path: /v1/readers/{n}/mifare/{block}
	if len(parts) < 5 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing block number (use /mifare/{block})",
		})
		return
	}

	blockNum, err := strconv.Atoi(parts[4])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid block number",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Read block
		// Optional query params: key (hex), keyType (A/B)
		key, err := parseMifareKey(r.URL.Query().Get("key"))
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}
		keyType := parseMifareKeyType(r.URL.Query().Get("keyType"))

		data, err := core.ReadMifareBlock(readerName, blockNum, key, keyType)
		if err != nil {
			logging.Debug(logging.CatHTTP, "MIFARE read failed", map[string]any{
				"reader": readerName,
				"block":  blockNum,
				"error":  err.Error(),
			})
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"block": blockNum,
			"data":  hex.EncodeToString(data),
		})

	case http.MethodPost:
		// Write block
		var req struct {
			Data    string `json:"data"`    // Hex string, 32 chars = 16 bytes
			Key     string `json:"key"`     // Optional, hex string, 12 chars = 6 bytes
			KeyType string `json:"keyType"` // Optional, "A" or "B"
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
			return
		}

		// Parse data
		data, err := hex.DecodeString(req.Data)
		if err != nil || len(data) != 16 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid data (must be 32 hex characters for 16 bytes)",
			})
			return
		}

		// Parse optional key
		key, err := parseMifareKey(req.Key)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}
		keyType := parseMifareKeyType(req.KeyType)

		if err := core.WriteMifareBlock(readerName, blockNum, data, key, keyType); err != nil {
			logging.Debug(logging.CatHTTP, "MIFARE write failed", map[string]any{
				"reader": readerName,
				"block":  blockNum,
				"error":  err.Error(),
			})
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]bool{
			"success": true,
		})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// handleUltralightPage handles read/write operations on MIFARE Ultralight pages
// GET /v1/readers/{n}/ultralight/{page} - Read page
// POST /v1/readers/{n}/ultralight/{page} - Write page
func handleUltralightPage(w http.ResponseWriter, r *http.Request, readerName string, parts []string) {
	// Expect path: /v1/readers/{n}/ultralight/{page}
	if len(parts) < 5 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing page number (use /ultralight/{page})",
		})
		return
	}

	pageNum, err := strconv.Atoi(parts[4])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid page number",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Read page
		password, err := parseUltralightPassword(r.URL.Query().Get("password"))
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		data, err := core.ReadUltralightPage(readerName, pageNum, password)
		if err != nil {
			logging.Debug(logging.CatHTTP, "Ultralight read failed", map[string]any{
				"reader": readerName,
				"page":   pageNum,
				"error":  err.Error(),
			})
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"page": pageNum,
			"data": hex.EncodeToString(data),
		})

	case http.MethodPost:
		// Write page
		var req struct {
			Data     string `json:"data"`     // Hex string, 8 chars = 4 bytes
			Password string `json:"password"` // Optional, hex string, 8 chars = 4 bytes
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		data, err := hex.DecodeString(req.Data)
		if err != nil || len(data) != 4 {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid data (must be 8 hex characters for 4 bytes)",
			})
			return
		}

		password, err := parseUltralightPassword(req.Password)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		if err := core.WriteUltralightPage(readerName, pageNum, data, password); err != nil {
			logging.Debug(logging.CatHTTP, "Ultralight write failed", map[string]any{
				"reader": readerName,
				"page":   pageNum,
				"error":  err.Error(),
			})
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]bool{"success": true})

	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// parseUltralightPassword parses an optional hex password string (8 hex chars = 4 bytes)
func parseUltralightPassword(pwdHex string) ([]byte, error) {
	if pwdHex == "" {
		return nil, nil
	}
	pwd, err := hex.DecodeString(pwdHex)
	if err != nil || len(pwd) != 4 {
		return nil, fmt.Errorf("password must be 8 hex characters (4 bytes)")
	}
	return pwd, nil
}

