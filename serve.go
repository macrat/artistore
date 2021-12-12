package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	InternalServerErrorMessage = "Internal server error.\nPlease check server log if you are server administrator."

	ErrInvalidRangeType = errors.New("Unsupported range length type. Please use bytes.")
	ErrInvalidRange     = errors.New("Requested range is not satisfiable.")
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start Artistore server",
	Long:  "Start Artistore server.",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		sec, err := GetSecret()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		s := Server{
			Secret: sec,
			Store: LocalStore{
				viper.GetString("store"),
				RetainPolicy{viper.GetInt("retain-num"), viper.GetDuration("retain-period")},
			},
		}

		PrintLog("INFO", "Starting Artistore on %s", viper.GetString("listen"))

		s.StartSweeper(5 * time.Minute)
		http.ListenAndServe(viper.GetString("listen"), gziphandler.GzipHandler(s))
	},
}

func init() {
	cmd.AddCommand(serveCmd)

	serveCmd.Flags().String("secret", "", "Server secret. See also 'artistore help secret'.")
	viper.BindPFlag("secret", serveCmd.Flags().Lookup("secret"))

	serveCmd.Flags().StringP("listen", "l", ":3000", "Listen address.")
	viper.BindPFlag("listen", serveCmd.Flags().Lookup("listen"))

	serveCmd.Flags().String("store", "/var/lib/artistore", "Path to data directory.")
	viper.BindPFlag("store", serveCmd.Flags().Lookup("store"))

	serveCmd.Flags().Int("retain-num", 0, "Number of to retain old revisions. (default retain all)")
	viper.BindPFlag("retain-num", serveCmd.Flags().Lookup("retain-num"))

	serveCmd.Flags().Duration("retain-period", 0, "Period of to retain old revisions. (default retain forever)")
	viper.BindPFlag("retain-period", serveCmd.Flags().Lookup("retain-period"))
}

type Server struct {
	Secret Secret
	Store  Store
}

func (s Server) StartSweeper(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				go s.Store.Sweep()
			}
		}
	}()
}

func (s Server) pathTo(key string, revision int) string {
	return "/" + key + "?rev=" + strconv.Itoa(revision)
}

type HeadWriter struct {
	w http.ResponseWriter
}

func (w HeadWriter) Header() http.Header {
	return w.w.Header()
}

func (w HeadWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w HeadWriter) WriteHeader(code int) {
	w.w.WriteHeader(code)
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	PrintLog(r.Method, "%s %s", r.RequestURI, r.RemoteAddr)

	w.Header().Set("Server", "Artistore")

	key := strings.TrimLeft(r.URL.Path, "/")
	if key == "" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "Please specify the key of artifact.")
		return
	}

	switch r.Method {
	case "GET":
		s.Get(key, w, r)
	case "POST":
		s.Post(key, w, r)
	case "HEAD":
		s.Get(key, HeadWriter{w}, r)
	case "OPTIONS":
		s.Options(key, w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintln(w, "Method not allowed.")
	}
}

type RangeRequest struct {
	From   int64
	To     int64
	Suffix int64
	Total  int64
}

func parseRangeRequest(header string) (req RangeRequest, err error) {
	if header == "" {
		return
	}
	if !strings.HasPrefix(header, "bytes=") {
		err = ErrInvalidRangeType
		return
	}
	header = strings.TrimSpace(header[len("bytes="):])
	header = strings.TrimSpace(strings.SplitN(header, ",", 2)[0])
	xs := strings.SplitN(header, "-", 2)

	if xs[0] != "" {
		req.From, err = strconv.ParseInt(xs[0], 10, 64)
		if err != nil {
			return
		}
		if xs[1] != "" {
			req.To, err = strconv.ParseInt(xs[1], 10, 64)
			if err != nil {
				return
			}
			req.To++
		}
	} else if xs[1] != "" {
		req.Suffix, err = strconv.ParseInt(xs[1], 10, 64)
		if err != nil {
			return
		}
	}

	if req.From < 0 || (req.To != 0 && req.To <= req.From) {
		err = ErrInvalidRange
	}

	return
}

func (req RangeRequest) String() string {
	to := req.To
	if to == 0 {
		to = req.Total
	}
	return fmt.Sprintf("bytes %d-%d/%d", req.From, to-1, req.Total)
}

func (req RangeRequest) Requested() bool {
	return req.From != 0 || req.To != 0 || req.Suffix != 0
}

func (req RangeRequest) Size() int64 {
	if req.Suffix > 0 {
		return req.Suffix
	}
	if req.To == 0 {
		return req.Total - req.From
	} else {
		return req.To - req.From
	}
}

func (s Server) Get(key string, w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("rev") {
		rev, err := strconv.Atoi(r.URL.Query().Get("rev"))
		if err != nil || rev < 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "Invalid revision.")
			return
		}

		meta, err := s.Store.Metadata(key, rev)
		if err == ErrNoSuchArtifact {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, err)
			return
		} else if err == ErrRevisionDeleted {
			w.WriteHeader(http.StatusGone)
			fmt.Fprintln(w, err)
			return
		} else if err != nil {
			PrintErr("ERROR", "%s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
			return
		}

		rangeRequest, err := parseRangeRequest(r.Header.Get("Range"))
		if err == ErrInvalidRangeType {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, err)
			return
		} else if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "Invalid range request.")
			return
		}

		f, meta, err := s.Store.Get(key, rev, rangeRequest)
		if err == ErrInvalidRangeType {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			fmt.Fprintln(w, err)
			return
		} else if err != nil {
			PrintErr("ERROR", "%s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
			return
		}
		defer f.Close()

		if hash := r.Header.Get("If-Range"); hash != "" {
			mod, err := http.ParseTime(hash)
			if (err == nil && meta.Timestamp.After(mod)) || (err != nil && meta.Hash != hash) {
				rangeRequest = RangeRequest{}
			}
		}

		rangeRequest.Total = meta.Size

		w.Header().Set("Content-Type", meta.Type)
		w.Header().Set("Etag", meta.Hash)
		w.Header().Set("Last-Modified", meta.Timestamp.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		if rangeRequest.Requested() {
			w.Header().Set("Content-Length", strconv.FormatInt(rangeRequest.Size(), 10))
			w.Header().Set("Content-Range", rangeRequest.String())
		} else {
			w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
		}
		w.Header().Set("Accept-Ranges", "bytes")

		if hash := r.Header.Get("If-None-Match"); hash == meta.Hash {
			w.WriteHeader(http.StatusNotModified)
			return
		} else if since := r.Header.Get("If-Modified-Since"); hash == "" && since != "" {
			t, err := http.ParseTime(since)
			if err == nil && meta.Timestamp.After(t) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		if _, ok := w.(HeadWriter); ok {
			return
		}

		if rangeRequest.Requested() {
			w.WriteHeader(http.StatusPartialContent)
			_, err = io.CopyN(w, f, rangeRequest.Size())
		} else {
			_, err = io.Copy(w, f)
		}
		if err != nil {
			PrintErr("ERROR", "%s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
		}
	} else {
		rev, err := s.Store.Latest(key)
		if err == ErrNoSuchArtifact {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, err)
		} else if err != nil {
			PrintErr("ERROR", "%s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
		} else {
			path := s.pathTo(key, rev)
			w.Header().Set("Location", path)
			w.WriteHeader(http.StatusSeeOther)
			fmt.Fprintln(w, "http://"+r.Host+path)
		}
	}
}

func (s Server) Post(key string, w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	auth := r.Header.Get("Authorization")
	if auth == "" {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, "Authorization header is required to publish artifact.")
		return
	} else if !strings.HasPrefix(auth, "bearer ") {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, "Authorization type should be bearer.")
		return
	} else if token, err := ParseToken(strings.TrimSpace(auth[len("bearer "):])); err != nil || !IsCorrentToken(s.Secret, token, key) {
		PrintWarn("FORBIDDEN", "%s %s", key, r.RemoteAddr)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, "Invalid authorization token.")
		return
	}

	rev, err := s.Store.Put(key, r.Body)
	if err != nil {
		PrintErr("ERROR", "%s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, InternalServerErrorMessage)
		return
	}

	PrintImportant("PUBLISH", "%s#%d", key, rev)

	w.Header().Set("Location", s.pathTo(key, rev))
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintln(w, "http://"+r.Host+s.pathTo(key, rev))
}

func (s Server) Options(key string, w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("rev") {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	} else {
		w.Header().Set("Allow", "GET, POST, HEAD, OPTIONS")
	}
}
