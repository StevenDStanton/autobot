// Package yt handles YouTube OAuth and video upload. All credentials are
// plain values from config.json, no side files.
package yt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	youtube "google.golang.org/api/youtube/v3"
)

// oauthConfig builds the OAuth client for the Desktop-app credentials the
// user copied into config.json.
func oauthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{youtube.YoutubeUploadScope},
	}
}

// Authorize runs the one-time local browser flow and returns the refresh
// token. Run on a machine with a browser; the caller stores the token in
// config.json.
func Authorize(ctx context.Context, clientID, clientSecret string) (string, error) {
	cfg := oauthConfig(clientID, clientSecret)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	cfg.RedirectURL = fmt.Sprintf("http://%s/callback", ln.Addr().String())

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	state := hex.EncodeToString(stateBytes)

	// offline + consent force Google to issue a refresh token even if the
	// app was authorized before.
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))

	// The Google URL is long enough that copying it from a terminal
	// reliably loses characters to line-wrapping. So the local server
	// forwards to it: the user only ever opens a short address.
	shortURL := fmt.Sprintf("http://%s/", ln.Addr().String())
	fmt.Println("Open this URL in your browser to authorize YouTube uploads:")
	fmt.Println("\n  " + shortURL + "\n")
	fmt.Println("(it forwards you to Google's sign-in page)")
	// #nosec G204 -- fixed binary, URL is built from our own listener address
	if err := exec.Command("xdg-open", shortURL).Start(); err != nil {
		fmt.Println("could not open a browser automatically:", err)
	}

	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, authURL, http.StatusFound)
			return
		}
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if got := q.Get("state"); got != state {
			msg := fmt.Sprintf(
				"state mismatch (got %d chars, expected %d): this usually means the URL was "+
					"copied from the terminal incompletely (line-wrapping dropped a character) "+
					"or an old browser tab was used. Run `autobotgo auth` again and use the tab "+
					"it opens, or copy the ENTIRE url.", len(got), len(state))
			http.Error(w, msg, http.StatusBadRequest)
			ch <- result{err: fmt.Errorf("oauth %s", msg)}
			return
		}
		if errMsg := q.Get("error"); errMsg != "" {
			http.Error(w, "authorization denied: "+errMsg, http.StatusBadRequest)
			ch <- result{err: fmt.Errorf("authorization denied: %s", errMsg)}
			return
		}
		fmt.Fprintln(w, "autobotgo is authorized. You can close this tab.")
		ch <- result{code: q.Get("code")}
	})}
	// Serve always ends with an error, normally ErrServerClosed from the
	// deferred Close below, so there is nothing useful to report here.
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	var res result
	select {
	case res = <-ch:
	case <-time.After(5 * time.Minute):
		return "", errors.New("timed out waiting for browser authorization")
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if res.err != nil {
		return "", res.err
	}

	tok, err := cfg.Exchange(ctx, res.code)
	if err != nil {
		return "", fmt.Errorf("exchanging authorization code: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", errors.New("no refresh token came back from Google; remove the app's access at myaccount.google.com/permissions and re-run")
	}
	return tok.RefreshToken, nil
}

// TokenSource returns an auto-refreshing token source built from the
// stored refresh token. Google does not rotate refresh tokens on normal
// refresh grants, so nothing needs writing back.
func TokenSource(ctx context.Context, clientID, clientSecret, refreshToken string) oauth2.TokenSource {
	cfg := oauthConfig(clientID, clientSecret)
	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
}
