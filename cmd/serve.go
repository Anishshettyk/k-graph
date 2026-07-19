package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	internalapi "github.com/anishetty/kgraph/internal/api"
)

var uiFiles embed.FS

// SetUIFiles is called from main with the embedded ui/dist filesystem.
func SetUIFiles(f embed.FS) { uiFiles = f }

var (
	servePort int
	serveOpen bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Launch the KGraph web UI at http://localhost:PORT",
	Long: `Starts a local HTTP server and opens the browser to the KGraph web UI.
The UI provides a visual interface for all kgraph commands, reading your
existing kubeconfig. Binds to 127.0.0.1 only — never exposed to the network.`,
	Args: cobra.NoArgs,
	RunE: runServe,
}

func runServe(_ *cobra.Command, _ []string) error {
	mux := http.NewServeMux()

	// API — delegates to the same internal packages as the CLI.
	mux.Handle("/api/", internalapi.New(kubeconfig))

	// Embedded React UI.
	sub, err := fs.Sub(uiFiles, "ui/dist")
	if err != nil {
		return fmt.Errorf("access embedded UI: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	addr := fmt.Sprintf("127.0.0.1:%d", servePort)
	url := fmt.Sprintf("http://localhost:%d", servePort)
	fmt.Fprintf(os.Stdout, "\n  ⬡  KGraph UI  →  %s\n", url)
	fmt.Fprintf(os.Stdout, "  kubeconfig: %s\n", kubeconfig)
	fmt.Fprintf(os.Stdout, "  Press Ctrl+C to stop\n\n")

	if serveOpen {
		go openBrowser(url)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	return srv.ListenAndServe()
}

func openBrowser(url string) {
	time.Sleep(600 * time.Millisecond)
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	exec.Command(cmd, args...).Start() //nolint:errcheck
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 7329, "port to listen on (default 7329)")
	serveCmd.Flags().BoolVar(&serveOpen, "open", true, "open browser automatically")
	rootCmd.AddCommand(serveCmd)
}
