package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"cobra/assets"
	"cobra/internal/state"
)

var watchPort int

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "serve the live dashboard: model steps, cage actions, context metrics",
	Long: `watch serves a zero-dependency web dashboard for this project on
localhost. It reads only the cage's own records (events.jsonl, state.json,
report.json) — the same binary-observed facts the strike system uses. It
never talks to the model and changes nothing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		mux := watchMux(dir)

		addr := fmt.Sprintf(":%d", watchPort)
		fmt.Printf("cage watch — dashboard at http://0.0.0.0%s/ (reading %s)\n", addr, state.EventsPath(dir))
		return http.ListenAndServe(addr, mux)
	},
}

// watchMux builds the dashboard handlers for a project dir. Shared by
// `cage watch` and the background dashboard launched by `cage run`.
func watchMux(dir string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(assets.DashboardHTML)
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		events, _ := state.ReadEventsTail(state.EventsPath(dir), 150)
		st, _ := state.Load(state.StatePath(dir))
		var report json.RawMessage
		if data, err := os.ReadFile(state.ReportPath(dir)); err == nil {
			report = data
		}
		payload := map[string]any{
			"events": events,
			"report": report,
		}
		if st != nil {
			payload["strikes"] = st.Strikes
			payload["max_strikes"] = state.MaxStrikes
			payload["locked"] = st.Locked
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	return mux
}

// startWatchServer launches the dashboard in the background using the same
// handlers as `cage watch`. It returns the dashboard URL and a shutdown func.
// quiet suppresses any terminal output. A busy port or listen failure never
// aborts the run: the URL reports it and shutdown is a no-op. Port 0 picks a
// free port.
func startWatchServer(dir string, port int, quiet bool) (string, func()) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if !quiet {
			fmt.Fprintf(os.Stderr, "dashboard unavailable: %v\n", err)
		}
		return "(dashboard unavailable)", func() {}
	}
	srv := &http.Server{Handler: watchMux(dir)}
	go func() { _ = srv.Serve(ln) }()
	url := fmt.Sprintf("http://%s/", ln.Addr().String())
	return url, func() { _ = srv.Close() }
}

func init() {
	watchCmd.Flags().IntVar(&watchPort, "port", 8060, "dashboard port")
	rootCmd.AddCommand(watchCmd)
}
