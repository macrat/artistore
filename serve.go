package main

import (
	"fmt"
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

		w.Header().Set("Content-Type", meta.Type)

		f, meta, err := s.Store.Get(key, rev)
		if err != nil {
			PrintErr("ERROR", "%s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
		}
		defer f.Close()

		w.Header().Set("Etag", `"`+meta.Hash+`"`)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		if _, ok := w.(HeadWriter); ok {
			return
		}

		http.ServeContent(w, r, meta.Key, meta.Timestamp, f)
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
			w.WriteHeader(http.StatusFound)
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
