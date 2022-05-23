package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type DropboxAccessResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	AccountID   string `json:"account_id"`
	TeamID      string `json:"team_id"`
	UID         string `json:"uid"`
}

const (
	dropboxIntegrationsURL      = "dropbox"
	dropboxClientID             = "rdx7fmqh73zemj7"
	dropboxRedirectURI          = "https://connect.onehcm.com/callbacks/dropbox"
	dropboxTokenDomain          = "https://api.dropboxapi.com"
	dropboxTokenURLResourcePath = "/oauth2/token"
	dropboxAccessGrantType      = "authorization_code"
	dropboxRefreshGrantType     = "refresh_token"
)

func dropboxAccessRequest(authCode string, companyIntegrationID int64) (OAuthTokens, error) {
	apiUrl := dropboxTokenDomain
	resource := dropboxTokenURLResourcePath
	data := url.Values{}
	data.Add("client_id", dropboxClientID)
	data.Add("code", authCode)
	data.Add("grant_type", boxAccessGrantType)
	data.Add("client_secret", passwords.DROPBOX_API_SECRET)
	data.Add("redirect_uri", dropboxRedirectURI)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		ErrorLog.Println("dropboxAccessRequest err: ", err)
		return OAuthTokens{}, errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("dropboxAccessRequest err: ", err)
		return OAuthTokens{}, errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println(bodyString)
		ErrorLog.Println("dropboxAccessRequest err with code: ", resp.StatusCode)
		return OAuthTokens{}, errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	boxResponse := DropboxAccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&boxResponse)
	if err != nil {
		ErrorLog.Println("dropboxAccessRequest NewDecoder err: ", err)
		return OAuthTokens{}, err
	}

	newOauthTokens := OAuthTokens{
		AccessToken:          boxResponse.AccessToken,
		RefreshToken:         "",
		Expires:              math.MaxInt64, //dropbox doesnt expire -- but can revoke
		CompanyIntegrationID: companyIntegrationID,
	}

	return newOauthTokens, nil
}
