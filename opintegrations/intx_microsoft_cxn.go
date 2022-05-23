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

type M365AccessResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}

type M365RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

const m365IntegrationsURL = "microsoft365"

const (
	mClientID    = "31706e82-d7e2-4aa3-a175-f255d059595b"
	mScope       = "User.ReadWrite.All Directory.Read.All offline_access"
	mGrantType   = "authorization_code"
	mRedirectURI = "https://connect.onehcm.com/callbacks/microsoft365"
)

func m365AccessRequest(authCode string, companyIntegrationID int64) (OAuthTokens, error) {
	apiUrl := "https://login.microsoftonline.com"
	resource := "/common/oauth2/v2.0/token"
	data := url.Values{}
	data.Add("client_id", mClientID)
	data.Add("scope", mScope)
	data.Add("code", authCode)
	data.Add("redirect_uri", mRedirectURI)
	data.Add("grant_type", mGrantType)
	data.Add("client_secret", passwords.M365_API_SECRET)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		ErrorLog.Println("m365 err: ", err)
		return OAuthTokens{}, errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("m365 err: ", err)
		return OAuthTokens{}, errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)
		// gSuiteResponse := GSuiteAccessErrorResponse{}
		// err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
		// if err != nil {
		// 	ErrorLog.Println("m365 NewDecoder err: ", err)
		// 	return OAuthTokens{}, err
		// }

		ErrorLog.Println(bodyString)
		ErrorLog.Println("m365 err with code: ", resp.StatusCode)
		return OAuthTokens{}, errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	m365Response := M365AccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&m365Response)
	if err != nil {
		ErrorLog.Println("m365 NewDecoder err: ", err)
		return OAuthTokens{}, err
	}

	newOauthTokens := OAuthTokens{
		AccessToken:          m365Response.AccessToken,
		RefreshToken:         m365Response.RefreshToken,
		Expires:              time.Now().Unix() + int64(m365Response.ExpiresIn),
		CompanyIntegrationID: companyIntegrationID,
	}

	return newOauthTokens, nil
}

func m365RefreshToken(authTokens OAuthTokens) (string, error) {
	if authTokens.RefreshToken == "" {
		refreshError("expired", authTokens)

		return "", errors.New("No refresh token to use, must reauthorize")
	}

	apiUrl := "https://login.microsoftonline.com"
	resource := "/common/oauth2/v2.0/token"
	data := url.Values{}
	data.Add("client_id", mClientID)
	data.Add("scope", mScope)
	data.Add("refresh_token", authTokens.RefreshToken)
	data.Add("redirect_uri", mRedirectURI)
	data.Add("grant_type", "refresh_token")
	data.Add("client_secret", passwords.M365_API_SECRET)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		// refreshError("expired", authTokens)

		ErrorLog.Println("m365 refresh err: ", err)
		return "", errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// refreshError("expired", authTokens)

		ErrorLog.Println("m365 refresh err: ", err)
		return "", errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		refreshError("expired", authTokens)

		ErrorLog.Println("m365 refresh err: ", bodyString, " with code: ", resp.StatusCode)
		return "", errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	m365Response := M365RefreshResponse{}
	err = json.NewDecoder(resp.Body).Decode(&m365Response)
	if err != nil {
		refreshError("expired", authTokens)

		ErrorLog.Println("m365 refresh NewDecoder err: ", err)
		return "", err
	}

	if m365Response.AccessToken == "" {
		refreshError("expired", authTokens)

		ErrorLog.Printf("m365 refresh did not send access token, response: %+v\n", m365Response)
		return "", errors.New("Could not refresh token")
	}

	authTokens.AccessToken = m365Response.AccessToken
	authTokens.Expires = time.Now().Unix() + int64(m365Response.ExpiresIn)
	_, err = dbmap.Update(&authTokens)
	if err != nil {
		ErrorLog.Println("could not update authToken err: ", err)
	}

	return m365Response.AccessToken, nil
}
