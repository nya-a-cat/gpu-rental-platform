package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Metrics struct {
	startedAt time.Time
	requests  atomic.Uint64
	inFlight  atomic.Int64
	panics    atomic.Uint64
}

func NewMetrics() *Metrics {
	return &Metrics{startedAt: time.Now()}
}

func (metrics *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		metrics.requests.Add(1)
		metrics.inFlight.Add(1)
		defer metrics.inFlight.Add(-1)
		next.ServeHTTP(response, request)
	})
}

func (metrics *Metrics) RecordPanic() {
	metrics.panics.Add(1)
}

func (metrics *Metrics) Handler(info SystemInfo) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(response, "# HELP gpu_control_plane_build_info Static build information.\n")
		_, _ = fmt.Fprintf(response, "# TYPE gpu_control_plane_build_info gauge\n")
		_, _ = fmt.Fprintf(response,
			"gpu_control_plane_build_info{version=%s,commit=%s,stage=%s} 1\n",
			prometheusLabel(info.Version),
			prometheusLabel(info.Commit),
			prometheusLabel(info.Stage),
		)
		_, _ = fmt.Fprintf(response, "# HELP gpu_control_plane_http_requests_total HTTP requests observed by this process.\n")
		_, _ = fmt.Fprintf(response, "# TYPE gpu_control_plane_http_requests_total counter\n")
		_, _ = fmt.Fprintf(response, "gpu_control_plane_http_requests_total %d\n", metrics.requests.Load())
		_, _ = fmt.Fprintf(response, "# HELP gpu_control_plane_http_requests_in_flight HTTP requests currently executing.\n")
		_, _ = fmt.Fprintf(response, "# TYPE gpu_control_plane_http_requests_in_flight gauge\n")
		_, _ = fmt.Fprintf(response, "gpu_control_plane_http_requests_in_flight %d\n", metrics.inFlight.Load())
		_, _ = fmt.Fprintf(response, "# HELP gpu_control_plane_panics_total Recovered HTTP handler panics.\n")
		_, _ = fmt.Fprintf(response, "# TYPE gpu_control_plane_panics_total counter\n")
		_, _ = fmt.Fprintf(response, "gpu_control_plane_panics_total %d\n", metrics.panics.Load())
		_, _ = fmt.Fprintf(response, "# HELP gpu_control_plane_process_uptime_seconds Process uptime in seconds.\n")
		_, _ = fmt.Fprintf(response, "# TYPE gpu_control_plane_process_uptime_seconds gauge\n")
		_, _ = fmt.Fprintf(response, "gpu_control_plane_process_uptime_seconds %s\n",
			strconv.FormatFloat(time.Since(metrics.startedAt).Seconds(), 'f', 3, 64),
		)
	})
}

func prometheusLabel(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
	return "\"" + replacer.Replace(value) + "\""
}
