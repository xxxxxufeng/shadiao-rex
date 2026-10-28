package api

import (
	"log"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	code  int
	bytes int64
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// AccessLog logs one line per request with the token query parameter removed.
func AccessLog(l *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		q := r.URL.Query()
		q.Del("t")
		path := r.URL.Path
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		l.Printf("%s %s %s %d %dB %s range=%q ua=%q", r.RemoteAddr, r.Method, path, sw.code, sw.bytes,
			time.Since(start).Round(time.Millisecond), r.Header.Get("Range"), r.UserAgent())
	})
}
