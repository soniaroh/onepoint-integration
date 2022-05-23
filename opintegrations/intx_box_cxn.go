package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type BoxAccessResponse struct {
	AccessToken  string        `json:"access_token"`
	ExpiresIn    int           `json:"expires_in"`
	TokenType    string        `json:"token_type"`
	RefreshToken string        `json:"refresh_token"`
	RestrictedTo []interface{} `json:"restricted_to"`
}

type BoxRefreshResponse struct {
	AccessToken string `json:"access_token"`
	InstanceURL string `json:"instance_url"`
	IssuedAt    string `json:"issued_at"`
}

const (
	boxIntegrationsURL      = "box"
	boxClientID             = "9vy3ki2waf1soyx1mky7sb27ifzm09nl"
	boxRedirectURI          = "https://connect.onehcm.com/callbacks/box"
	boxTokenDomain          = "https://api.box.com"
	boxTokenURLResourcePath = "/oauth2/token"
	boxAccessGrantType      = "authorization_code"
	boxRefreshGrantType     = "refresh_token"
	SECSINDAY               = 86400
)

func boxAccessRequest(authCode string, companyIntegrationID int64) (OAuthTokens, error) {
	apiUrl := boxTokenDomain
	resource := boxTokenURLResourcePath
	data := url.Values{}
	data.Add("client_id", boxClientID)
	data.Add("code", authCode)
	data.Add("grant_type", boxAccessGrantType)
	data.Add("client_secret", passwords.BOX_API_SECRET)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		ErrorLog.Println("box err: ", err)
		return OAuthTokens{}, errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("box err: ", err)
		return OAuthTokens{}, errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println(bodyString)
		ErrorLog.Println("box err with code: ", resp.StatusCode)
		return OAuthTokens{}, errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	boxResponse := BoxAccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&boxResponse)
	if err != nil {
		ErrorLog.Println("sf NewDecoder err: ", err)
		return OAuthTokens{}, err
	}

	newOauthTokens := OAuthTokens{
		AccessToken:          boxResponse.AccessToken,
		RefreshToken:         boxResponse.RefreshToken,
		Expires:              time.Now().Unix() + int64(boxResponse.ExpiresIn),
		CompanyIntegrationID: companyIntegrationID,
	}

	return newOauthTokens, nil
}

// Box API is a little different, refresh tokens are onetime use, also expire after 60 days
func boxRefreshToken(authTokens OAuthTokens) (string, error) {
	if authTokens.RefreshToken == "" || time.Now().Unix() > (authTokens.Expires+int64(SECSINDAY*60)) {
		refreshError("expired", authTokens)

		return "", errors.New("No refresh token to use, must reauthorize")
	}

	apiUrl := "https://www.box.com"
	resource := "/api/oauth2/token"
	data := url.Values{}
	data.Add("client_id", boxClientID)
	data.Add("refresh_token", authTokens.RefreshToken)
	data.Add("grant_type", "refresh_token")
	data.Add("client_secret", passwords.BOX_API_SECRET)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		ErrorLog.Println("box err: ", err)
		return "", errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("box err: ", err)
		return "", errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println(bodyString)
		ErrorLog.Println("box err with code: ", resp.StatusCode)
		return "", errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	boxResponse := BoxAccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&boxResponse)
	if err != nil {
		ErrorLog.Println("sf NewDecoder err: ", err)
		return "", err
	}

	newOauthTokens := OAuthTokens{
		ID:                   authTokens.ID,
		AccessToken:          boxResponse.AccessToken,
		RefreshToken:         boxResponse.RefreshToken,
		Expires:              time.Now().Unix() + int64(boxResponse.ExpiresIn),
		CompanyIntegrationID: authTokens.CompanyIntegrationID,
		Active:               authTokens.Active,
	}

	_, err = dbmap.Update(&newOauthTokens)
	if err != nil {
		ErrorLog.Println("could not update authToken err: ", err)
	}

	return newOauthTokens.AccessToken, nil
}
