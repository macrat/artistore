package main

import (
	"fmt"
	"time"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

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

		log.Print("Starting Artistore on ", viper.GetString("listen"))

		s.StartSweeper(5*time.Minute)
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

		for  {
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

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL)
	switch r.Method {
	case "GET":
		s.Get(w, r)
	case "POST":
		s.Post(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintln(w, "Method not allowed.")
	}
}

func (s Server) Get(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimLeft(r.URL.Path, "/")
	if key == "" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "Please specify the key of artifact.")
	} else if rev, err := strconv.Atoi(r.URL.Query().Get("rev")); err == nil && rev > 0 {
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
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
			return
		}

		w.Header().Set("Content-Type", meta.Type)

		err = s.Store.Get(w, key, rev)
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, InternalServerErrorMessage)
		}
	} else {
		rev, err = s.Store.Latest(key)
		if err == ErrNoSuchArtifact {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, err)
		} else if err != nil {
			log.Print(err)
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

func (s Server) Post(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	key := strings.TrimLeft(r.URL.Path, "/")
	if key == "" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintln(w, "Method not allowed.")
		return
	}

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
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, "Invalid authorization token.")
		return
	}

	rev, err := s.Store.Put(key, r.Body)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, InternalServerErrorMessage)
		return
	}

	fmt.Fprintln(w, "http://"+r.Host+s.pathTo(key, rev))
}
