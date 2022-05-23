package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type GSuiteAccessResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}

type GSuiteAccessErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type GoogleAccessRequest struct {
	Code         string `json:"code"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	GrantType    string `json:"grant_type"`
}

type GSuiteRefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type GSuiteRefreshRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	GrantType    string `json:"grant_type"`
}

type GSuiteInfo struct {
	AccessURL   string
	Secret      string
	ClientID    string
	RedirectURI string
}

var gsuiteInfo = GSuiteInfo{
	AccessURL:   "https://www.googleapis.com/oauth2/v4/token",
	ClientID:    "296350640338-ltb2c3lmaeg9ucs1hcen6tru20j73uje.apps.googleusercontent.com",
	RedirectURI: fmt.Sprintf("https://connect.onehcm.com/callbacks/%s", gSuiteIntegrationsURL),
}

const gSuiteIntegrationsURL = "gsuite"

func gSuiteAccessRequest(authCode string, companyIntegrationID int64) (OAuthTokens, error) {
	request := GoogleAccessRequest{
		Code:         authCode,
		ClientID:     gsuiteInfo.ClientID,
		ClientSecret: passwords.GSUITE_API_SECRET,
		RedirectURI:  gsuiteInfo.RedirectURI,
		GrantType:    "authorization_code",
	}

	return googleOAuthRequest(&request, companyIntegrationID)
}

// Used for both GSuite (user provisioning) and Google Hire
func googleOAuthRequest(request *GoogleAccessRequest, companyIntegrationID int64) (OAuthTokens, error) {
	apiUrl := "https://www.googleapis.com"
	resource := "/oauth2/v4/token"
	data := url.Values{}
	data.Set("code", request.Code)
	data.Add("client_id", request.ClientID)
	data.Add("client_secret", request.ClientSecret)
	data.Add("redirect_uri", request.RedirectURI)
	data.Add("grant_type", "authorization_code")

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		ErrorLog.Println("google err: ", err)
		return OAuthTokens{}, errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("google err: ", err)
		return OAuthTokens{}, errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		gSuiteResponse := GSuiteAccessErrorResponse{}
		err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
		if err != nil {
			ErrorLog.Println("google NewDecoder err: ", err)
			return OAuthTokens{}, err
		}

		ErrorLog.Println("google err: ", gSuiteResponse.ErrorDescription, " with code: ", resp.StatusCode)
		return OAuthTokens{}, errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	gSuiteResponse := GSuiteAccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
	if err != nil {
		ErrorLog.Println("google NewDecoder err: ", err)
		return OAuthTokens{}, err
	}

	newOauthTokens := OAuthTokens{
		AccessToken:          gSuiteResponse.AccessToken,
		RefreshToken:         gSuiteResponse.RefreshToken,
		Expires:              time.Now().Unix() + int64(gSuiteResponse.ExpiresIn),
		CompanyIntegrationID: companyIntegrationID,
	}

	return newOauthTokens, nil
}

func gSuiteRefreshToken(request GoogleAccessRequest, authTokens *OAuthTokens) (string, error) {
	if authTokens.RefreshToken == "" {
		refreshError("expired", *authTokens)

		return "", errors.New("No refresh token to use, must reauthorize")
	}

	apiUrl := "https://www.googleapis.com"
	resource := "/oauth2/v4/token"
	data := url.Values{}
	data.Add("client_id", request.ClientID)
	data.Add("client_secret", request.ClientSecret)
	data.Add("grant_type", request.GrantType)
	data.Add("refresh_token", authTokens.RefreshToken)

	u, _ := url.ParseRequestURI(apiUrl)
	u.Path = resource
	urlStr := u.String()

	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
	if err != nil {
		// refreshError("expired", authTokens)

		ErrorLog.Println("google err: ", err)
		return "", errors.New("Something went wrong")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// refreshError("expired", authTokens)

		ErrorLog.Println("google err: ", err)
		return "", errors.New("Something is wrong on our end")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		gSuiteResponse := GSuiteAccessErrorResponse{}
		err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
		if err != nil {
			refreshError("expired", *authTokens)

			ErrorLog.Println("google NewDecoder err: ", err)
			return "", err
		}

		refreshError("expired", *authTokens)

		ErrorLog.Println("google err: ", gSuiteResponse.ErrorDescription, " with code: ", resp.StatusCode)
		return "", errors.New("BAD REQUEST: " + strconv.Itoa(resp.StatusCode))
	}

	gSuiteResponse := GSuiteRefreshResponse{}
	err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
	if err != nil {
		refreshError("expired", *authTokens)

		ErrorLog.Println("google NewDecoder err: ", err)
		return "", err
	}

	if gSuiteResponse.AccessToken == "" {
		refreshError("expired", *authTokens)

		ErrorLog.Printf("refresh did not send access token, response: %+v\n", gSuiteResponse)
		return "", errors.New("Could not refresh token")
	}

	authTokens.AccessToken = gSuiteResponse.AccessToken
	authTokens.Expires = time.Now().Unix() + int64(gSuiteResponse.ExpiresIn)
	_, err = dbmap.Update(authTokens)
	if err != nil {
		ErrorLog.Println("could not update authToken err: ", err)
	}

	return gSuiteResponse.AccessToken, nil
}
