package main

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type OAuthCodeRequest struct {
	IntegrationURL string `json:"name"`
	Code           string `json:"code"`
}

func registerOauthTokenRoutes(router *gin.Engine) {
	router.POST("/api/oauth/code", handleNewCode)
}

func handleNewCode(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	input := &OAuthCodeRequest{}
	if err := c.ShouldBindWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input is wrong format"})
		return
	}

	if input.IntegrationURL == GOOGLE_HIRE_URL {
		googleHireOAuthCodeHandler(&thisCompany, input, c)
		return
	}

	thisIntegration := Integration{}
	err = dbmap.SelectOne(&thisIntegration, "SELECT * FROM integrations WHERE url = ?", input.IntegrationURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid integration"})
		return
	}

	thisCompanyIntegration, err := getCompanyIntegration(thisIntegration.ID, thisCompany.ID)
	if err != nil {
		ErrorLog.Println("err looking up CompanyIntegration: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
		return
	}

	// if this comp is already connected to this int... either block this or inactivate that
	existingActiveOauthToken := OAuthTokens{}
	err = dbmap.SelectOne(&existingActiveOauthToken, "SELECT * FROM oauth_tokens WHERE company_integration_id = ? AND active = 1", thisCompanyIntegration.ID)
	if err == nil {
		InfoLog.Printf("new UP connection but there is existingActiveOauthToken for ci: %+v\n", thisCompanyIntegration)
		falseBool := false
		existingActiveOauthToken.Active = &falseBool
		_, err = dbmap.Update(&existingActiveOauthToken)
		if err != nil {
			ErrorLog.Println("err updating existingActiveOauthToken active: ", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
			return
		}

		thisCompanyIntegration.AuthedUsername = ""
		thisCompanyIntegration.Status = "new"
		_, err = dbmap.Update(&thisCompanyIntegration)
		if err != nil {
			ErrorLog.Println("err updating thisCompanyIntegration: ", err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
			return
		}
	}

	authTokens, err := sendAuthToken(input.Code, thisIntegration, &thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
		return
	}

	userName, err := getUserName(authTokens.AccessToken, thisIntegration, thisCompanyIntegration)
	if err != nil || userName == "" {
		ErrorLog.Println("err looking up oauth username: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection, please try connecting using a different account."})
		return
	}

	err = checkAccess(userName, authTokens.AccessToken, thisIntegration)
	if err != nil {
		if err.Error() == "403" {
			ErrorLog.Println("returning error to user because we deem non access")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User does not have enough access to perform necessary actions, please have another admin connect."})
			return
		} else {
			ErrorLog.Println("checkAccess error but still allowing user to connect:", userName, ", access token: ", authTokens.AccessToken)
		}
	}

	trueVar := true
	authTokens.Active = &trueVar

	err = dbmap.Insert(&authTokens)
	if err != nil {
		ErrorLog.Println("err inserting tokens: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
		return
	}

	thisCompanyIntegration.Status = "connected"
	thisCompanyIntegration.AuthedUsername = userName

	_, err = dbmap.Update(&thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("err updating CompanyIntegration: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
		return
	}

	publicCI := transformCIToPublic(thisCompanyIntegration)

	c.JSON(http.StatusOK, publicCI)
}

func sendAuthToken(authCode string, thisIntegration Integration, thisCoIntegration *CompanyIntegration) (OAuthTokens, error) {
	var authTokens OAuthTokens
	var err error

	switch thisIntegration.URL {
	case gSuiteIntegrationsURL:
		authTokens, err = gSuiteAccessRequest(authCode, thisCoIntegration.ID)
	case m365IntegrationsURL:
		authTokens, err = m365AccessRequest(authCode, thisCoIntegration.ID)
	case boxIntegrationsURL:
		authTokens, err = boxAccessRequest(authCode, thisCoIntegration.ID)
	case dropboxIntegrationsURL:
		authTokens, err = dropboxAccessRequest(authCode, thisCoIntegration.ID)
	case sfIntegrationsURL:
		apiURL := ""
		sfUserID := ""
		authTokens, apiURL, sfUserID, err = sfAccessRequest(authCode, thisCoIntegration.ID)
		if err != nil {
			return authTokens, err
		}

		thisCoIntegration.Settings["api_url"] = apiURL
		thisCoIntegration.Settings["sf_id"] = sfUserID
	default:
		err = errors.New("Unknown integration")
	}

	return authTokens, err
}

func getUserName(accessToken string, thisIntegration Integration, thisCoIntegration CompanyIntegration) (string, error) {
	var err error
	userName := ""

	switch thisIntegration.URL {
	case gSuiteIntegrationsURL:
		userName, err = getGSuiteUserName(accessToken)
	case m365IntegrationsURL:
		userName, err = getM365Name(accessToken)
	case boxIntegrationsURL:
		userName, err = getBoxName(accessToken)
	case dropboxIntegrationsURL:
		userName, err = getDropboxName(accessToken)
	case sfIntegrationsURL:
		userName, err = getSFName(accessToken, thisCoIntegration)
	default:
		err = errors.New("Unknown integration")
	}

	return userName, err
}

func checkAccess(emailAddress string, accessToken string, thisIntegration Integration) error {
	var err error

	switch thisIntegration.URL {
	case gSuiteIntegrationsURL:
		err = gSuiteTestAccess(emailAddress, accessToken)
	case boxIntegrationsURL:
		err = boxCheckAccess(accessToken)
	case dropboxIntegrationsURL:
		err = dropboxCheckAccess(emailAddress)
	case m365IntegrationsURL:
		// m365 blocks non admins from logging in :)
	case sfIntegrationsURL:
		// TODO!
	default:
		err = errors.New("Unknown integration")
	}

	return err
}
