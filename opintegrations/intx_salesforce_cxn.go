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

type SFAccessResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	InstanceURL  string `json:"instance_url"`
	IssuedAt     string `json:"issued_at"`
	UserID       string `json:"id"`
}

type SFRefreshResponse struct {
	AccessToken string `json:"access_token"`
	InstanceURL string `json:"instance_url"`
	IssuedAt    string `json:"issued_at"`
}

const sfIntegrationsURL = "salesforce"

const (
	sfClientID    = "3MVG9CEn_O3jvv0yMoqoYfA3RC_7sm49y45ZNr._yIK7nWKT7_jtTz2rC6IyS6qbJo2wzhRqQ2awHbkxQsIJp"
	sfRedirectURI = "https://connect.onehcm.com/callbacks/salesforce"
	tokenDomain   = "https://login.salesforce.com"
	// tokenDomain        = "https://test.salesforce.com"
	tokenResourcePath  = "/services/oauth2/token"
	sfAccessGrantType  = "authorization_code"
	sfRefreshGrantType = "refresh_token"
	DEFAULT_SF_TIMEOUT = 1000
)

func sfAccessRequest(authCode string, companyIntegrationID int64) (OAuthTokens, string, string, error) {
	apiUrl := tokenDomain
	resource := tokenResourcePath
	data := url.Values{}
	data.Add("client_id", sfClientID)
	data.Add("code", authCode)
	data.Add("redirect_uri", sfRedirectURI)
	data.Add("grant_type", sfAccessGrantType)
	data.Add("client_secret", passwords.SF_API_SECRET)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		ErrorLog.Println("sf err: ", err)
		return OAuthTokens{}, "", "", errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("sf err: ", err)
		return OAuthTokens{}, "", "", errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println(bodyString)
		ErrorLog.Println("sf err with code: ", resp.StatusCode)
		return OAuthTokens{}, "", "", errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	sfResponse := SFAccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&sfResponse)
	if err != nil {
		ErrorLog.Println("sf NewDecoder err: ", err)
		return OAuthTokens{}, "", "", err
	}

	newOauthTokens := OAuthTokens{
		AccessToken:          sfResponse.AccessToken,
		RefreshToken:         sfResponse.RefreshToken,
		CompanyIntegrationID: companyIntegrationID,
		Expires:              time.Now().Unix() + DEFAULT_SF_TIMEOUT,
	}

	return newOauthTokens, sfResponse.InstanceURL, sfResponse.UserID, nil
}

func sfRefreshToken(authTokens OAuthTokens) (string, error) {
	if authTokens.RefreshToken == "" {
		refreshError("expired", authTokens)

		return "", errors.New("No refresh token to use, must reauthorize")
	}

	apiUrl := tokenDomain
	resource := tokenResourcePath
	data := url.Values{}
	data.Add("grant_type", sfRefreshGrantType)
	data.Add("client_id", sfClientID)
	data.Add("refresh_token", authTokens.RefreshToken)
	data.Add("redirect_uri", sfRedirectURI)
	data.Add("client_secret", passwords.SF_API_SECRET)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		// refreshError("expired", authTokens)

		ErrorLog.Println("sf refresh err: ", err)
		return "", errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// refreshError("expired", authTokens)

		ErrorLog.Println("sf refresh err: ", err)
		return "", errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		refreshError("expired", authTokens)

		ErrorLog.Println("sf refresh err: ", bodyString, " with code: ", resp.StatusCode)
		return "", errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	sfResponse := SFRefreshResponse{}
	err = json.NewDecoder(resp.Body).Decode(&sfResponse)
	if err != nil {
		refreshError("expired", authTokens)

		ErrorLog.Println("sf refresh NewDecoder err: ", err)
		return "", err
	}

	if sfResponse.AccessToken == "" {
		refreshError("expired", authTokens)

		ErrorLog.Printf("sf refresh did not send access token, response: %+v\n", sfResponse)
		return "", errors.New("Could not refresh token")
	}

	authTokens.AccessToken = sfResponse.AccessToken
	authTokens.Expires = time.Now().Unix() + DEFAULT_SF_TIMEOUT
	_, err = dbmap.Update(&authTokens)
	if err != nil {
		ErrorLog.Println("could not update authToken err: ", err)
	}

	return sfResponse.AccessToken, nil
}
