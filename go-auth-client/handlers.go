package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/sessions"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"learn.oauth.client/model"
)

const (
	AuthCodeKey     = "AuthCode"
	SessionStateKey = "SessionState"
	AccessTokenKey  = "AccessToken"
)

type frontData struct {
	SessionState string
	Token        map[string]interface{}
	Services     []string
}

var t = template.Must(template.ParseFiles("src/template/index.html"))
var tServices = template.Must(template.ParseFiles("src/template/index.html", "src/template/services.html"))
var store = sessions.NewCookieStore([]byte("your-secret-key"))
var tokenResponse model.TokenResponseData

func homeHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")

	data := frontData{
		SessionState: getSessionValue(session, SessionStateKey),
		Token:        tokenResponseToMap(tokenResponse),
	}

	err := t.Execute(w, data)
	if err != nil {
		log.Println("Template execution error:", err)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request, appVar *config) {
	redirectURL := buildAuthURL(appVar)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func logoutHandler(w http.ResponseWriter, r *http.Request, appVar *config) {
	session, _ := store.Get(r, "session-name")

	delete(session.Values, AuthCodeKey)
	delete(session.Values, SessionStateKey)

	err := session.Save(r, w)
	if err != nil {
		log.Println("Error saving session:", err)
	}

	redirectURL := buildLogoutURL(appVar)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func authCodeRedirectHandler(w http.ResponseWriter, r *http.Request, appVar *config) {
	session, _ := store.Get(r, "session-name")

	authCode := r.URL.Query().Get("code")
	sessionState := r.URL.Query().Get("session_state")

	// Save auth code and session state in the session
	session.Values[AuthCodeKey] = authCode
	session.Values[SessionStateKey] = sessionState
	session.Save(r, w)

	// Exchange auth code for token
	token, err := exchangeAuthCodeForToken(authCode, appVar)
	if err != nil {
		log.Println("Error exchanging auth code for token:", err)
		http.Error(w, "Failed to exchange authorization code for token", http.StatusInternalServerError)
		return
	}

	// Save token in the session
	session.Values[AccessTokenKey] = token
	err = session.Save(r, w)
	if err != nil {
		log.Println("Error saving session:", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func servicesHandler(w http.ResponseWriter, r *http.Request, appVar *config) {
	req, err := http.NewRequest("GET", appVar.ServicesURL, nil)
	if err != nil {
		log.Println("Error creating a new HTTP request:", err)
		return
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()

	c := http.Client{}
	res, err := c.Do(req.WithContext(ctx))
	if err != nil {
		log.Println("Error sending HTTP request:", err)
		return
	}

	byteBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println("Error reading response body:", err)
		return
	}

	billingResponse := &model.Billing{}
	err = json.Unmarshal(byteBody, billingResponse)
	if err != nil {
		log.Println("Error unmarshalling JSON response:", err)
		return
	}

	session, _ := store.Get(r, "session-name")
	data := frontData{
		SessionState: getSessionValue(session, SessionStateKey),
		Token:        tokenResponseToMap(tokenResponse),
		Services:     billingResponse.Services,
	}

	err = tServices.Execute(w, data)
	if err != nil {
		log.Println("Template execution error:", err)
	}
}

func exchangeAuthCodeForToken(authCode string, appVar *config) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", appVar.AppID)
	data.Set("client_secret", "1ANIYGdYJhdeMjXOn6qrSmMU9wiUkXQ2")
	data.Set("code", authCode)
	data.Set("redirect_uri", appVar.AuthCodeCallback)

	req, err := http.NewRequest("POST", appVar.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return "", fmt.Errorf("token request returned status code %d. Response body: %s", resp.StatusCode, responseBody)
	}

	err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
	if err != nil {
		return "", err
	}

	return tokenResponse.AccessToken, nil
}

func buildAuthURL(appVar *config) string {
	u, err := url.Parse(appVar.AuthURL)
	if err != nil {
		log.Println(err)
		return ""
	}

	qs := u.Query()
	qs.Add("state", "test_state")
	qs.Add("client_id", appVar.AppID)
	qs.Add("response_type", "code")
	qs.Add("redirect_uri", appVar.AuthCodeCallback)
	u.RawQuery = qs.Encode()

	return u.String()
}

func buildLogoutURL(appVar *config) string {
	u, err := url.Parse(appVar.LogoutURL)
	if err != nil {
		log.Println(err)
		return ""
	}

	q := u.Query()
	q.Add("redirect_uri", appVar.LogoutRedirect)
	u.RawQuery = q.Encode()

	return u.String()
}

func getSessionValue(session *sessions.Session, key string) string {
	value := session.Values[key]
	if value != nil {
		return value.(string)
	}
	return ""
}

func tokenResponseToMap(response model.TokenResponseData) map[string]interface{} {
	data := make(map[string]interface{})
	data["AccessToken"] = response.AccessToken
	data["TokenType"] = response.TokenType
	data["ExpiresIn"] = response.ExpiresIn
	data["RefreshToken"] = response.RefreshToken
	data["Scope"] = response.Scope
	return data
}