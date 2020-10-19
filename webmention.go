package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	units "github.com/docker/go-units"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"

	"github.com/jcgregorio/go-lib/admin"
	"github.com/jcgregorio/logger"
	"github.com/jcgregorio/webmention-run/mention"
)

// Config keys as found in config.json.
const (
	DATASTORE_NAMESPACE = "DATASTORE_NAMESPACE"
	CLIENT_ID           = "CLIENT_ID"
	REGION              = "REGION"
	PROJECT             = "PROJECT"
	ADMINS              = "ADMINS"
	HOST                = "HOST"
	AUTHOR              = "AUTHOR"
	TARGETS             = "TARGETS"
)

// flags
var (
	local        = flag.Bool("local", false, "Running locally if true. As opposed to in production.")
	resourcesDir = flag.String("resources_dir", "", "The directory to find templates, JS, and CSS files. If blank the current directory will be used.")
)

var (
	m *mention.Mentions

	log = logger.New()

	ad *admin.Admin

	triageTemplate *template.Template

	mentionsTemplate *template.Template
)

func initialize() {
	flag.Parse()
	viper.SetConfigType("json")
	if *resourcesDir == "" {
		_, filename, _, _ := runtime.Caller(0)
		*resourcesDir = filepath.Join(filepath.Dir(filename))
	}
	f, err := os.Open(filepath.Join(*resourcesDir, "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := viper.ReadConfig(f); err != nil {
		log.Fatal(err)
	}
	log.Infof("%q\n", *resourcesDir)

	ad = admin.New(viper.GetString(CLIENT_ID), viper.GetStringSlice(ADMINS))

	triageTemplate = template.Must(template.New("triage").Funcs(template.FuncMap{
		"trunc": func(s string) string {
			if len(s) > 80 {
				return s[:80] + "..."
			}
			return s
		},
		"humanTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return " • " + units.HumanDuration(time.Now().Sub(t)) + " ago"
		},
	}).Parse(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title></title>
    <meta charset="utf-8" />
    <meta http-equiv="X-UA-Compatible" content="IE=egde,chrome=1">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="google-signin-scope" content="profile email">
    <meta name="google-signin-client_id" content="%s">
    <script src="https://apis.google.com/js/platform.js" async defer></script>
		<style type="text/css" media="screen">
		  #webmentions {
				display: grid;
				padding: 1em;
				grid-template-columns: 5em 10em 1fr;
				grid-column-gap: 10px;
				grid-row-gap: 6px;
			}
		</style>
</head>
<body>
  <div class="g-signin2" data-onsuccess="onSignIn" data-theme="dark"></div>
    <script>
      function onSignIn(googleUser) {
        document.cookie = "id_token=" + googleUser.getAuthResponse().id_token;
        if (!{{.IsAdmin}}) {
          window.location.reload();
        }
      };
    </script>
  <div id=webmentions>
  {{range .Mentions }}
		<select name="text" data-key="{{ .Key }}">
			<option value="good" {{if eq .State "good" }}selected{{ end }} >Good</option>
			<option value="spam" {{if eq .State "spam" }}selected{{ end }} >Spam</option>
			<option value="untriaged" {{if eq .State "untriaged" }}selected{{ end }} >Untriaged</option>
		</select>
		<span>{{ .TS | humanTime }}</span>
		<div>
		  <div>Source: <a href="{{ .Source }}">{{ .Source | trunc }}</a></div>
			<div>Target: <a href="{{ .Target }}">{{ .Target | trunc }}</a></div>
		</div>
  {{end}}
  </div>
	<div><a href="?offset={{.Offset}}">Next</a></div>
	<script type="text/javascript" charset="utf-8">
	 // TODO - listen on div.webmentions for click/input and then write
	 // triage action back to server.
	 document.getElementById('webmentions').addEventListener('change', e => {
		 console.log(e);
		 if (e.target.dataset.key != "") {
			 fetch("/UpdateMention", {
			   credentials: 'same-origin',
				 method: 'POST',
				 body: JSON.stringify({
					 key: e.target.dataset.key,
					 value:  e.target.value,
				 }),
				 headers: new Headers({
					 'Content-Type': 'application/json'
				 })
			 }).catch(e => console.error('Error:', e));
		 }
	 });
	</script>
</body>
</html>`, viper.GetString(CLIENT_ID))))

	mentionsTemplate = template.Must(template.New("mentions").Funcs(template.FuncMap{
		"humanTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return " • " + units.HumanDuration(time.Now().Sub(t)) + " ago"
		},
		"rfc3999": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format(time.RFC3339)
		},
		"trunc": func(s string) string {
			if len(s) > 200 {
				return s[:200] + "..."
			}
			return s
		},
	}).Parse(`
	<section id=webmention>
	<h3>WebMentions</h3>
	{{ $host := .Host }}
	{{ range .Mentions }}
			<span class="wm-author">
				{{ if .AuthorURL }}
					{{ if .Thumbnail }}
					<a href="{{ .AuthorURL}}" rel=nofollow class="wm-thumbnail">
						<img src="{{ $host }}/Thumbnail/{{ .Thumbnail }}"/>
					</a>
					{{ end }}
					<a href="{{ .AuthorURL}}" rel=nofollow>
						{{ .Author }}
					</a>
				{{ else }}
					{{ .Author }}
				{{ end }}
			</span>
			<time datetime="{{ .TS | rfc3999 }}">{{ .TS | humanTime }}</time>
				{{ if .URL }}
			    <a class="wm-content" href="{{ .URL }}" rel=nofollow>
				{{ else }}
			    <a class="wm-content" href="{{ .Source }}" rel=nofollow>
				{{ end }}
				{{ if .Title }}
					{{ .Title | trunc }}
				{{ else }}
					{{ .Source | trunc }}
				{{ end }}
			</a>
	{{ end }}
	</section>
`))

	m, err = mention.NewMentions(context.Background(), viper.GetString(PROJECT), viper.GetString(DATASTORE_NAMESPACE), log)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Info("Initialized.")
	}
}

type triageContext struct {
	IsAdmin  bool
	Mentions []*mention.MentionWithKey
	Offset   int64
}

// triageHandler displays the triage page for Webmentions.
func triageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	context := &triageContext{}
	isAdmin := ad.IsAdmin(r, log)
	if isAdmin {
		limitText := r.FormValue("limit")
		if limitText == "" {
			limitText = "20"
		}
		offsetText := r.FormValue("offset")
		if offsetText == "" {
			offsetText = "0"
		}
		limit, err := strconv.ParseInt(limitText, 10, 32)
		if err != nil {
			log.Infof("Failed to parse limit: %s", err)
			return
		}
		offset, err := strconv.ParseInt(offsetText, 10, 32)
		if err != nil {
			log.Infof("Failed to parse offset: %s", err)
			return
		}
		context = &triageContext{
			IsAdmin:  isAdmin,
			Mentions: m.GetTriage(r.Context(), int(limit), int(offset)),
			Offset:   offset + limit,
		}
	}
	if err := triageTemplate.Execute(w, context); err != nil {
		log.Errorf("Failed to render triage template: %s", err)
	}
}

type updateMention struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// updateMentionHandler updates the triage state of a webmention.
// Called from the Triage page.
func updateMentionHandler(w http.ResponseWriter, r *http.Request) {
	isAdmin := ad.IsAdmin(r, log)
	if !isAdmin {
		http.Error(w, "Unauthorized", 401)
	}
	var u updateMention
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		log.Infof("Failed to decode update: %s", err)
		http.Error(w, "Bad JSON", 400)
	}
	if err := m.UpdateState(r.Context(), u.Key, u.Value); err != nil {
		log.Infof("Failed to write update: %s", err)
		http.Error(w, "Failed to write", 400)
	}
}

// MentionsContext is the data for expanding the Mentions template.
type MentionsContext struct {
	Host     string
	Mentions []*mention.Mention
}

// mentionsHandler returns HTML describing all the good Webmentions for the given URL.
func mentionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Referrer-Policy", "unsafe-url")
	if r.Method == "OPTIONS" {
		return
	}
	ref := r.Referer()
	if len(ref) == 0 {
		return
	}
	if strings.HasSuffix(ref, "/") {
		ref = ref[:len(ref)-1]
	}
	mentions := m.GetGood(r.Context(), ref)
	if len(mentions) == 0 {
		return
	}
	context := MentionsContext{
		Host:     viper.GetString(HOST),
		Mentions: mentions,
	}
	if err := mentionsTemplate.Execute(w, context); err != nil {
		log.Errorf("Failed to expand template: %s", err)
	}
}

// incomingWebMentionHandler handles incoming Webmentions.
func incomingWebMentionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mention := mention.New(r.FormValue("source"), r.FormValue("target"))
	if err := mention.FastValidate(viper.GetStringSlice(TARGETS)); err != nil {
		log.Infof("Invalid request: %s", err)
		http.Error(w, fmt.Sprintf("Invalid request."), 400)
		return
	}
	if err := m.Put(r.Context(), mention); err != nil {
		log.Infof("Failed to enqueue mention: %s", err)
		http.Error(w, fmt.Sprintf("Failed to enqueue mention."), 400)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func thumbnailHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	vars := mux.Vars(r)
	b, err := m.GetThumbnail(r.Context(), vars["id"])
	if err != nil {
		http.Error(w, "Image not found", 404)
		log.Warningf("Failed to get image: %s", err)
		return
	}
	if _, err = w.Write(b); err != nil {
		log.Errorf("Failed to write image: %s", err)
		return
	}
}

// verifyQueuedMentions verifies untriaged webmentions.
//
// Should be called on a timer.
func verifyQueuedMentions(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{
		Timeout: time.Second * 30,
	}
	m.VerifyQueuedMentions(client)
}

func main() {
	initialize()

	r := mux.NewRouter()
	r.HandleFunc("/Mentions", mentionsHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/IncomingWebMention", incomingWebMentionHandler).Methods("POST")
	r.HandleFunc("/UpdateMention", updateMentionHandler).Methods("POST")
	r.HandleFunc("/Thumbnail/{id:[a-z0-9]+}", thumbnailHandler).Methods("GET")
	r.HandleFunc("/VerifyQueuedMentions", verifyQueuedMentions).Methods("POST")
	r.HandleFunc("/", triageHandler).Methods("GET")

	http.Handle("/", r)
	port := os.Getenv("PORT")
	if port == "" {
		port = "1313"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
