package main

import (
	"errors"
	"fmt"
	"time"
)

type OAuthTokens struct {
	ID                   int64  `db:"id, primarykey, autoincrement"`
	CompanyIntegrationID int64  `db:"company_integration_id"`
	AccessToken          string `db:"access_token"`
	RefreshToken         string `db:"refresh_token"`
	Expires              int64  `db:"expires"`
	Active               *bool  `db:"active"`
}

func deleteOAuthByCompanyIntegrationID(ciID int64) error {
	thisToken := &OAuthTokens{}
	err := dbmap.SelectOne(thisToken, "SELECT * FROM oauth_tokens WHERE company_integration_id = ? AND active = 1", ciID)
	if err != nil {
		ErrorLog.Println("deleteOAuthByCompanyIntegrationID SelectOne: ", err)
		return errors.New("Could not find auth token to delete")
	}

	*thisToken.Active = false

	_, err = dbmap.Update(thisToken)
	if err != nil {
		ErrorLog.Println("deleteOAuthByCompanyIntegrationID Update: ", err)
		return errors.New("Could not update")
	}

	return nil
}

func refreshError(newConnectionStatus string, authTokens OAuthTokens) {
	thisCompanyIntegration := CompanyIntegration{}
	err := dbmap.SelectOne(&thisCompanyIntegration, "SELECT * FROM company_integrations WHERE id = ?", authTokens.CompanyIntegrationID)
	if err != nil {
		ErrorLog.Println("refreshError could not lookup company integration: ", err)
		return
	}

	thisCompanyIntegration.Status = newConnectionStatus
	dbmap.Update(&thisCompanyIntegration)

	authTokens.Expires = 0
	dbmap.Update(&authTokens)

	err = deleteOAuthByCompanyIntegrationID(thisCompanyIntegration.ID)
	if err != nil {
		ErrorLog.Println("refreshError could not delete auth token: ", err)
		return
	}

	// TODO:  send notification to the user to re authenticate
}

func getAccessToken(companyIntegration CompanyIntegration) (string, error) {
	thisAccessToken := OAuthTokens{}
	err := dbmap.SelectOne(&thisAccessToken, "SELECT * FROM oauth_tokens WHERE company_integration_id = ? AND active = 1", companyIntegration.ID)
	if err != nil {
		return "", errors.New(fmt.Sprintf("could not lookup an oauth token for %+v\n", companyIntegration))
	}

	nowUnixSeconds := time.Now().Unix()

	if thisAccessToken.Expires > nowUnixSeconds {
		return thisAccessToken.AccessToken, nil
	} else {
		newAccessToken, err := refreshToken(companyIntegration, thisAccessToken)
		if err != nil {
			// TODO: if this fails, send notification ?
			ErrorLog.Printf("REFRESH ERROR!: co: %v, err: %v\n", companyIntegration, err)
			return "", err
		}

		return newAccessToken, nil
	}
}

func refreshToken(companyIntegration CompanyIntegration, thisAccessToken OAuthTokens) (string, error) {
	integration, erry := getIntegrationByID(companyIntegration.IntegrationID)
	if erry != nil {
		return "", erry
	}

	var err error
	newAccessToken := ""

	switch integration.URL {
	case gSuiteIntegrationsURL:
		request := GoogleAccessRequest{
			ClientID:     gsuiteInfo.ClientID,
			ClientSecret: passwords.GSUITE_API_SECRET,
			GrantType:    "refresh_token",
		}
		newAccessToken, err = gSuiteRefreshToken(request, &thisAccessToken)
	case m365IntegrationsURL:
		newAccessToken, err = m365RefreshToken(thisAccessToken)
	case sfIntegrationsURL:
		newAccessToken, err = sfRefreshToken(thisAccessToken)
	case boxIntegrationsURL:
		newAccessToken, err = boxRefreshToken(thisAccessToken)
	default:
		err = errors.New("Unknown integration")
	}

	return newAccessToken, err
}

func getOAuthTokenByID(tokenID int64) (*OAuthTokens, error) {
	toks := &OAuthTokens{}
	err := dbmap.SelectOne(toks, "SELECT * FROM oauth_tokens WHERE id = ?", tokenID)
	return toks, err
}

func (t *OAuthTokens) insert() error {
	return dbmap.Insert(t)
}
