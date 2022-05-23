package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"opapi"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

type GoogleHireInfoResponse struct {
	AuthInfo          *GoogleHireAuthInfo           `json:"auth_info"`
	CreatedApplicants []GoogleHireCreatedApplicants `json:"created_applicants"`
}

type GoogleHireAuthInfo struct {
	ID                           int64        `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID                    int64        `db:"company_id" json:"company_id"`
	OAuthID                      *int64       `db:"oauth_id" json:"oauth_id"`
	Authenticated                bool         `db:"-" json:"authenticated"`
	OAuthToken                   *OAuthTokens `db:"-" json:"-"`
	NotificationRegistrationName string       `db:"notification_registration_name" json:"notification_registration_name"`
	LastFetched                  int64        `db:"last_fetched" json:"last_fetched"`
}

type GoogleHireCreatedApplicants struct {
	ID                      int64  `db:"id, primarykey, autoincrement" json:"id"`
	CompanyID               int64  `db:"company_id" json:"company_id"`
	GHApplicantResourceName string `db:"gh_applicant_resource_name" json:"gh_applicant_resource_name"`
	OPApplicantID           int64  `db:"op_id" json:"op_id"`
	OPApplicantUsername     string `db:"op_username" json:"op_username"`
	ApplicantName           string `db:"applicant_name" json:"applicant_name"`
	ApplicantEmail          string `db:"applicant_email" json:"applicant_email"`
	CreatedAt               int64  `db:"created_at" json:"created_at"`
}

const GOOGLE_HIRE_URL = "googlehire"
const GOOGLE_HIRE_REDIRECT = "https://connect.onehcm.com/callbacks/googlehire"
const GOOGLE_HIRE_REDIRECT_DEV = "http://localhost:3000/callbacks/googlehire"
const GH_CLIENTID = "296350640338-1vosi9tcli87utnsvvq9rooo17n11tmo.apps.googleusercontent.com"
const GH_SECRET = "P5VvL5S3H6wN_RFM9_DRlHMx"
const GH_PUSH_NOTIFICATION_TYPE = "APPLICATION_STATUS_CHANGE"
const EXWIRE_GH_STAGE = "BACKGROUND IN PROGRESS"

func registerGoogleHireRoutes(router *gin.Engine) {
	router.GET("/api/googlehire/info", getGoogleHireInfoHandler)
	router.POST("/api/googlehire/deauthenticate", deactivateGoogleHireHandler)
	router.POST("/api/googlehire/pushNotifications", googleHirePushNotificationsHandler)
}

// runs everyday using crons
func renewGoogleHireRegistrations() {
	ghs := getAllActiveGHAuths()
	for _, gh := range ghs {
		gh.fillInExtra()
		err := gh.CreateOrRenewGHNotifications()
		if err != nil {
			ErrorLog.Println("err CreateOrRenewGHNotifications co id: ", gh.CompanyID, " err: ", err)
		} else {
			InfoLog.Println("CreateOrRenewGHNotifications SUCCESS for co id: ", gh.CompanyID)
		}
	}
}

// this is an alternate to the push notifications method, currently not used
func runGoogleHireChangesPull() {
	allGHAuths := getAllActiveGHAuths()

	for _, ghAuth := range allGHAuths {
		ghAuth.fillInExtra()

		company, err := lookupCompanyByID(ghAuth.CompanyID)
		if err != nil {
			ErrorLog.Println("runGoogleHireChangesPull lookupCompanyByShortname err:", err)
			return
		}

		lastFetched := ghAuth.LastFetched
		if lastFetched == 0 {
			lastFetched = time.Now().Add(-(time.Minute * 1)).UTC().Unix()
		}

		lastFetchedTime := time.Unix(lastFetched, 0).UTC()
		timeFetchedString := lastFetchedTime.Format("2006-01-02T15:04:05Z")

		InfoLog.Println("starting runGoogleHireChangesPull for ", company.ShortName, " with time: ", timeFetchedString)

		baseURL := "https://hire.googleapis.com/v1beta1/tenants/my_tenant/applications"
		query := fmt.Sprintf("filter=status.state=ACTIVE AND status.update_time>=\"%s\"", timeFetchedString)
		query = url.PathEscape(query)

		result := GHApplicationListResult{}
		err = ghAuth.GHAPIQuery(fmt.Sprintf("%s?%s", baseURL, query), &result)
		if err != nil {
			ErrorLog.Println("runGoogleHireChangesPull GHAPIQuery err:", err)
			return
		}

		ghAuth.LastFetched = time.Now().UTC().Unix()
		dbmap.Update(&ghAuth)

		InfoLog.Println("runGoogleHireChangesPull got ", len(result.Applications), " applications")

		for _, app := range result.Applications {
			app.ProcessChangedApplication(EXWIRE_GH_STAGE, &ghAuth)
		}
	}
}

func (application *GHApplication) ProcessChangedApplication(desiredStage string, ghAuthInfo *GoogleHireAuthInfo) {
	logPrefix := "GoogleHire ProcessChangedApplication "

	if strings.ToUpper(application.Status.ProcessStage.Stage) != desiredStage {
		InfoLog.Println(logPrefix, "application in undesired stage: ", strings.ToUpper(application.Status.ProcessStage.Stage), ", application: ", application.Name)
		return
	}

	alreadyCreated := findGHCreatedApplicantsByGHNameExists(application.Candidate)

	if alreadyCreated {
		InfoLog.Println(logPrefix, "application in desired stage category but already created, applicant resource name: ", application.Candidate)
		return
	}

	ghCandidate, err := ghAuthInfo.FetchCandidate(application.Candidate)
	if err != nil {
		ErrorLog.Println(logPrefix, "ghAuthInfo.FetchCandidate err: ", err)
		return
	}

	if ghCandidate.Email == "" {
		ErrorLog.Println(logPrefix, "NEED TO CREATE APPLICANT BUT EMAIL WAS BLANK, candidate name = ", ghCandidate.Name)
		return
	}

	// only field required is country
	appAddress := &opapi.ApplicantAddress{Country: "USA"}
	if len(ghCandidate.PostalAddresses) > 0 {
		address := ghCandidate.PostalAddresses[0]
		if len(address.AddressLines) > 0 {
			appAddress.AddressLine1 = address.AddressLines[0]
			if len(address.AddressLines) > 1 {
				appAddress.AddressLine2 = address.AddressLines[1]
			}
		}
		appAddress.State = address.AdministrativeArea
		appAddress.Zip = address.PostalCode
		appAddress.City = address.Locality
		appAddress.State = address.AdministrativeArea
	}

	appPhone := &opapi.ApplicantPhones{}
	if ghCandidate.PhoneNumber != "" {
		appPhone.CellPhone = ghCandidate.PhoneNumber
		appPhone.PreferredPhone = "CELL"
	} else {
		appPhone = nil
	}

	newApplicant := opapi.ApplicantBasic{
		Account: opapi.ApplicantAccount{
			Username:            ghCandidate.Email,
			FirstName:           ghCandidate.PersonName.GivenName,
			LastName:            ghCandidate.PersonName.FamilyName,
			PrimaryEmail:        ghCandidate.Email,
			Locked:              false,
			ForceChangePassword: true,
		},
		Phones:  appPhone,
		Address: appAddress,
	}

	co, err := lookupCompanyByShortname("ZJARED")
	if env.Production {
		co, err = lookupCompanyByID(ghAuthInfo.CompanyID)
	}
	if err != nil {
		ErrorLog.Println(logPrefix, "lookupCompanyByID err: ", err)
		return
	}

	cxn := chooseOPAPICxn(co.OPID)

	err, opErr, _ := cxn.CreateApplicant(co.OPID, &newApplicant)
	if err != nil {
		ErrorLog.Println(logPrefix, "CreateApplicant err: ", err, " opERR: ", opErr)
		return
	}

	// not adding opid yet because it is returned in header, would need refactor
	// UPDATE TO ABOVE, can now read headers from response
	creationLog := &GoogleHireCreatedApplicants{
		CompanyID:               co.ID,
		GHApplicantResourceName: ghCandidate.Name,
		OPApplicantUsername:     newApplicant.Account.Username,
		ApplicantName:           fmt.Sprintf("%s %s", newApplicant.Account.FirstName, newApplicant.Account.LastName),
		ApplicantEmail:          newApplicant.Account.PrimaryEmail,
		CreatedAt:               time.Now().UTC().Unix(),
	}

	dbmap.Insert(creationLog)

	// loginLink := fmt.Sprintf("https://secure.onehcm.com/ta/%s.login", co.ShortName)

	// now sending through OnePoint only
	// creationLog.SendApplicantNotifications("P@ssw0rd", loginLink)
	creationLog.SendCompanyNotifications()
}

func googleHirePushNotificationsHandler(c *gin.Context) {
	/*
		if env.Production {
			bodyBytes, err := ioutil.ReadAll(c.Request.Body)
			if err != nil {
				ErrorLog.Println("googleHirePushNotificationsHandler err ReadAll err: ", err)
				return
			}
			bodyString := string(bodyBytes)

			InfoLog.Println("GOOGLE HIRE PUSH: ", bodyString)
			return
		}
	*/

	webhookBody := &GoogleHireWebhookBody{}
	if err := c.ShouldBindWith(webhookBody, binding.JSON); err != nil {
		ErrorLog.Println("err binding googleHirePushNotificationsHandler json: ", err)
		c.JSON(http.StatusOK, gin.H{"ack": "success"})
		return
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(webhookBody.Message.Data)
	if err != nil {
		ErrorLog.Println("err base64 data googleHirePushNotificationsHandler err: ", err)
		c.JSON(http.StatusOK, gin.H{"ack": "success"})
		return
	}

	msg := &GHWebhookMessageData{}
	err = json.Unmarshal(decodedBytes, msg)
	if err != nil {
		ErrorLog.Println("err json.Unmarshal googleHirePushNotificationsHandler err: ", err)
		c.JSON(http.StatusOK, gin.H{"ack": "success"})
		return
	}

	ghAuthInfo, err := lookupGHAuthInfoByRegName(msg.Registration)
	if err != nil {
		ErrorLog.Println("googleHirePushNotificationsHandler lookupGHAuthInfoByRegName err: ", err)
		c.JSON(http.StatusOK, gin.H{"ack": "success"})
		return
	}

	ghAuthInfo.fillInExtra()

	if msg.NotificationType != GH_PUSH_NOTIFICATION_TYPE {
		ErrorLog.Println("googleHirePushNotificationsHandler msg.NotificationType was diff type than expected: ", msg.NotificationType)
		c.JSON(http.StatusOK, gin.H{"ack": "success"})
		return
	}

	application, err := ghAuthInfo.FetchApplication(msg.Resource)
	if err != nil {
		ErrorLog.Println("googleHirePushNotificationsHandler ghAuthInfo.FetchApplication err: ", err)
		c.JSON(http.StatusOK, gin.H{"ack": "success"})
		return
	}

	go application.ProcessChangedApplication(EXWIRE_GH_STAGE, ghAuthInfo)

	c.JSON(http.StatusOK, gin.H{"ack": "success"})
}

func (createdLog *GoogleHireCreatedApplicants) SendApplicantNotifications(newPassword, loginLink string) {
	toStringArr := []string{"jaredshahbazian@gmail.com"}
	if env.Production {
		toStringArr = []string{createdLog.ApplicantEmail}
	}

	tos := []*sgmail.Email{}
	for _, toStr := range toStringArr {
		tos = append(tos, &sgmail.Email{Address: toStr})
	}

	emailHeaderInfo := sgEmailFields{
		Subject: "Exclusive Wireless Onboarding",
		From:    &sgmail.Email{Name: "Exclusive Wireless HR", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      tos,
		Bcc:     []*sgmail.Email{},
	}

	emailBody := GHApplicantAlertBody{
		Username:  createdLog.OPApplicantUsername,
		Password:  newPassword,
		LoginLink: loginLink,
	}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, GH_APPLICANT_EMAIL_TEMPLATE, emailBody, "google_hire_applicant_email")
	if err != nil {
		ErrorLog.Println("GOOGLE HIRE SendApplicantNotifications err: ", err)
	}
}

func (createdLog *GoogleHireCreatedApplicants) SendCompanyNotifications() {
	toStringArr := []string{"jaredshahbazian@gmail.com"}
	if env.Production {
		toStringArr = []string{"lizeth.campa@exclusivewireless.net", "danielle.amezola@exclusivewireless.net"}
	}

	tos := []*sgmail.Email{}
	for _, toStr := range toStringArr {
		tos = append(tos, &sgmail.Email{Address: toStr})
	}

	emailHeaderInfo := sgEmailFields{
		Subject: fmt.Sprintf("Applicant added to OnePoint from Google Hire: %s", createdLog.ApplicantName),
		From:    &sgmail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      tos,
	}

	emailBody := GHAdminAlertBody{ApplicantName: createdLog.ApplicantName}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, GH_ADMIN_ALERT_TEMPLATE, emailBody, "google_hire_admin_alerts")
	if err != nil {
		ErrorLog.Println("GOOGLE HIRE SendCompanyNotifications err: ", err)
	}
}

func deactivateGoogleHireHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	ghi, err := thisCompany.getGoogleHireAuthInfo()
	if err != nil {
		c.JSON(http.StatusOK, GoogleHireInfoResponse{})
		return
	}

	ghi.OAuthID = nil
	dbmap.Update(ghi)
	ghi.fillInExtra()

	ghi.DeleteGHNotifications()
	if err != nil {
		ErrorLog.Println("deactivateGoogleHireHandler worked but DeleteGHNotifications errd: ", err)
	}

	c.JSON(http.StatusOK, GoogleHireInfoResponse{AuthInfo: ghi})
}

func getGoogleHireInfoHandler(c *gin.Context) {
	thisCompany, err := lookupCompany(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not authorized"})
		return
	}

	ghi, err := thisCompany.getGoogleHireAuthInfo()
	if err != nil {
		ghi = nil
	}

	capps := []GoogleHireCreatedApplicants{}
	dbmap.Select(&capps, "SELECT * FROM google_hire_created_applicants WHERE company_id = ? ORDER BY created_at DESC LIMIT 200", thisCompany.ID)

	resp := GoogleHireInfoResponse{
		AuthInfo:          ghi,
		CreatedApplicants: capps,
	}

	c.JSON(http.StatusOK, resp)
}

func googleHireOAuthCodeHandler(company *Company, input *OAuthCodeRequest, c *gin.Context) {
	redirectURL := GOOGLE_HIRE_REDIRECT
	if !env.Production {
		redirectURL = GOOGLE_HIRE_REDIRECT_DEV
	}
	request := GoogleAccessRequest{
		Code:         input.Code,
		ClientID:     GH_CLIENTID,
		ClientSecret: GH_SECRET,
		RedirectURI:  redirectURL,
		GrantType:    "authorization_code",
	}

	newToks, err := googleOAuthRequest(&request, 0)
	if err != nil {
		ErrorLog.Println("googleOAuthRequest err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
		return
	}

	active := true
	newToks.Active = &active
	err = newToks.insert()
	if err != nil {
		ErrorLog.Println("newToks.insert err: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not authorize the connection."})
		return
	}

	authInfo, err := company.getGoogleHireAuthInfo()
	if err == nil {
		authInfo.OAuthID = &newToks.ID
		dbmap.Update(authInfo)
	} else {
		newAuthInfo := &GoogleHireAuthInfo{
			CompanyID: company.ID,
			OAuthID:   &newToks.ID,
		}
		dbmap.Insert(newAuthInfo)
		authInfo = newAuthInfo
	}

	authInfo.fillInExtra()

	err = authInfo.CreateOrRenewGHNotifications()
	if err != nil {
		ErrorLog.Println("GOOGLE AUTH WORKED BUT authInfo.CreateOrRenewGHNotifications ERRD: ", err)
	}

	c.JSON(http.StatusOK, authInfo)
}

func (existingGHAuthInfo *GoogleHireAuthInfo) CreateOrRenewGHNotifications() error {
	if existingGHAuthInfo == nil {
		return errors.New("auth is nil")
	}

	if existingGHAuthInfo.OAuthToken == nil {
		return errors.New("tokens is nil")
	}

	reqBody := GHNewRegRequest{
		Registration: GHRegistration{
			NotificationTypes: []string{GH_PUSH_NOTIFICATION_TYPE},
			PubsubTopic:       "projects/onepoint-integrations/topics/hire_changes",
		},
	}

	err := existingGHAuthInfo.GHCheckAndRefreshAccessToken()
	if err != nil {
		ErrorLog.Println("GHCheckAndRefreshAccessToken err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}

	urlStr := "https://hire.googleapis.com/v1alpha1/tenants/my_tenant/registrations"
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(reqBody)

	req, err := http.NewRequest("POST", urlStr, b)
	if err != nil {
		ErrorLog.Println("createOrRenewGHNotifications NewRequest err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+existingGHAuthInfo.OAuthToken.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("createOrRenewGHNotifications Do err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		bodyString := string(bodyBytes)

		ErrorLog.Println("createOrRenewGHNotifications request >300 status, body: ", bodyString)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}

	responseReg := GHRegistration{}
	err = json.NewDecoder(resp.Body).Decode(&responseReg)
	if err != nil {
		ErrorLog.Println("createOrRenewGHNotifications google NewDecoder err: ", err)
		return err
	}

	if responseReg.Name != nil {
		existingGHAuthInfo.NotificationRegistrationName = *responseReg.Name
	}
	dbmap.Update(existingGHAuthInfo)

	return nil
}

func (existingGHAuthInfo *GoogleHireAuthInfo) DeleteGHNotifications() error {
	if existingGHAuthInfo == nil {
		return errors.New("auth is nil")
	}

	if existingGHAuthInfo.NotificationRegistrationName == "" {
		return errors.New("NotificationRegistrationName is empty")
	}

	if existingGHAuthInfo.OAuthToken == nil {
		return errors.New("tokens is nil")
	}

	urlStr := "https://hire.googleapis.com/v1alpha1/" + existingGHAuthInfo.NotificationRegistrationName

	req, err := http.NewRequest("DELETE", urlStr, nil)
	if err != nil {
		return errors.New("DeleteGHNotifications NewRequest err: " + err.Error())
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("DeleteGHNotifications client.Do err: " + err.Error())
	}
	defer resp.Body.Close()

	existingGHAuthInfo.NotificationRegistrationName = ""
	dbmap.Update(existingGHAuthInfo)

	return nil
}

func (c *Company) getGoogleHireAuthInfo() (*GoogleHireAuthInfo, error) {
	ghi := &GoogleHireAuthInfo{}
	err := dbmap.SelectOne(ghi, "SELECT * FROM google_hire_auth WHERE company_id = ?", c.ID)
	if err == nil {
		ghi.fillInExtra()
	}
	return ghi, err
}

func lookupGHAuthInfoByRegName(regName string) (*GoogleHireAuthInfo, error) {
	ghi := &GoogleHireAuthInfo{}
	err := dbmap.SelectOne(ghi, "SELECT * FROM google_hire_auth WHERE notification_registration_name = ?", regName)
	if err == nil {
		ghi.fillInExtra()
	}
	return ghi, err
}

func (ghi *GoogleHireAuthInfo) fillInExtra() {
	ghi.Authenticated = false
	if ghi.OAuthID != nil {
		ghi.getOAuth()
		ghi.Authenticated = true
	}
}

func (ghi *GoogleHireAuthInfo) getOAuth() {
	if ghi.OAuthID != nil {
		tok, err := getOAuthTokenByID(*ghi.OAuthID)
		if err == nil {
			ghi.OAuthToken = tok
		}
	}
}

func (ghi *GoogleHireAuthInfo) GHCheckAndRefreshAccessToken() error {
	nowUnixSeconds := time.Now().Unix()

	if ghi.OAuthToken.Expires > nowUnixSeconds {
		return nil
	} else {
		err := ghi.GHRefreshToken()
		if err != nil {
			// TODO: if this fails, send notification ?
			ErrorLog.Printf("GoogleHireAuthInfo REFRESH ERROR!: co_id: %v, err: %v\n", ghi.CompanyID, err)
			return err
		}

		return nil
	}
}

func (ghi *GoogleHireAuthInfo) GHRefreshToken() error {
	if ghi.OAuthToken.RefreshToken == "" {
		return errors.New("refresh token empty!")
	}

	request := GoogleAccessRequest{
		ClientID:     GH_CLIENTID,
		ClientSecret: GH_SECRET,
		GrantType:    "refresh_token",
	}
	newAccess, err := gSuiteRefreshToken(request, ghi.OAuthToken)
	if err != nil {
		return err
	}

	ghi.OAuthToken.AccessToken = newAccess

	return nil
}

func getAllActiveGHAuths() []GoogleHireAuthInfo {
	ghs := []GoogleHireAuthInfo{}
	dbmap.Select(&ghs, "SELECT * FROM google_hire_auth WHERE !ISNULL(oauth_id)")
	return ghs
}

func (ghi *GoogleHireAuthInfo) FetchApplication(resourceName string) (*GHApplication, error) {
	urlBase := "https://hire.googleapis.com/v1beta1/"
	url := urlBase + resourceName
	output := &GHApplication{}
	err := ghi.GHAPIQuery(url, output)
	return output, err
}

func (ghi *GoogleHireAuthInfo) FetchCandidate(resourceName string) (*GHCandidate, error) {
	urlBase := "https://hire.googleapis.com/v1beta1/"
	url := urlBase + resourceName
	output := &GHCandidate{}
	err := ghi.GHAPIQuery(url, output)
	return output, err
}

func (ghi *GoogleHireAuthInfo) GHAPIQuery(url string, holder interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	err = ghi.GHCheckAndRefreshAccessToken()
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+ghi.OAuthToken.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return errors.New("GHAPIQuery err and could not parse returned body")
		}
		bodyString := string(bodyBytes)
		return errors.New("GHAPIQuery url: " + url + " err: " + bodyString)
	}

	err = json.NewDecoder(resp.Body).Decode(holder)
	if err != nil {
		return err
	}

	return nil
}

func findGHCreatedApplicantsByGHNameExists(GHResourceName string) bool {
	qry := "SELECT COUNT(*) FROM google_hire_created_applicants WHERE gh_applicant_resource_name = ?"
	count, err := dbmap.SelectInt(qry, GHResourceName)
	if err != nil {
		return false
	}
	return count > 0
}
