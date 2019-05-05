package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jcgregorio/slog"
	"github.com/jcgregorio/webmention-func/config"
)

type Claims struct {
	Mail    string `json:"email"`
	Aud     string `json:"aud"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

var (
	client = &http.Client{
		Timeout: time.Second * 30,
	}
)

func IsAdmin(r *http.Request, log slog.Logger) bool {
	idtoken, err := r.Cookie("id_token")
	if err != nil {
		log.Infof("No cookie supplied.")
		return false
	}
	resp, err := client.Get(fmt.Sprintf("https://www.googleapis.com/oauth2/v3/tokeninfo?%s", idtoken))
	if err != nil || resp.StatusCode != 200 {
		log.Infof("Failed to validate idtoken: %#v %s", *resp, err)
		return false
	}
	claims := Claims{}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		log.Infof("Failed to decode claims: %s", err)
		return false
	}
	// Check if aud is correct.
	if claims.Aud != config.CLIENT_ID {
		log.Infof("Wrong audience.")
		return false
	}

	for _, email := range config.ADMINS {
		if email == claims.Mail {
			return true
		}
	}
	log.Infof("%q is not an administrator.", claims.Mail)
	return false
}
