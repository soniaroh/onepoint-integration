package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"opapi"
)

func sfHired(newHiredEvent *HFEvent, opCompanyID int64, fullEmployeeRecord opapi.OPFullEmployeeRecord, resultsSoFar []HFCompanyIntegrationResultJoin) (string, string, HFCreatedCredentials) {
	var createdCredentials = HFCreatedCredentials{}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(sfIntegrationsURL, newHiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("salesforce couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}

	*newHiredEvent.Processed = true
	dbmap.Update(newHiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new hired but salesforce disabled")

		return HF_DISABLED_LABEL, "Salesforce Integration was disabled at the time of processing event.", createdCredentials
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new hired but not connected to salesforce")

		return HF_FAILED_LABEL, "Salesforce not connected.", createdCredentials
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("salesforce checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}
	if blockedByCC {
		ErrorLog.Println("salesforce was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The hired employee was not in one of the selected cost centers at time of hire.", createdCredentials
	}

	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("salesforce ADD getAccessToken err: ", err)

		return HF_FAILED_LABEL, "Could not authenticate with Salesforce, please reconnect.", createdCredentials
	}

	orgURL, ok := thisCompanyIntegration.Settings["api_url"].(string)
	if !ok {
		ErrorLog.Println("salesforce orgURL doesnt exist")

		return HF_FAILED_LABEL, "Could not authenticate with Salesforce, please reconnect.", createdCredentials
	}

	sfUserReq, err := constructNewSalesforceUser(accessToken, orgURL, opCompanyID, fullEmployeeRecord, thisCompanyIntegration, resultsSoFar)
	if err != nil {
		ErrorLog.Println("salesforce constructNewSalesforceUser err: ", err)

		return HF_FAILED_LABEL, err.Error(), createdCredentials
	}

	createdCredentials, err = sfCreateUser(accessToken, orgURL, sfUserReq)
	if err != nil {
		ErrorLog.Println("salesforce hired err: ", err)

		return HF_FAILED_LABEL, err.Error(), createdCredentials
	}

	InfoLog.Println("add salesforce user: ", createdCredentials.Username, ", successfully added to salesforce")

	return HF_SUCCESS_LABEL, "Added " + createdCredentials.Username + " to Salesforce.", createdCredentials
}

func sfFired(newFiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string) {
	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(sfIntegrationsURL, newFiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("sfFired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}

	*newFiredEvent.Processed = true
	dbmap.Update(newFiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new fired but salesforce disabled")

		return HF_DISABLED_LABEL, "Salesforce Integration was disabled at the time of processing event."
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new fired but not connected to salesforce")

		return HF_FAILED_LABEL, "Salesforce not connected."
	}

	if newFiredEvent.Email == "" {
		ErrorLog.Println("trying to remove sfFired user but theres no email address")

		return HF_FAILED_LABEL, "Could not delete from Salesforce because employee was terminated without an email address."
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("salesforce checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}
	if blockedByCC {
		ErrorLog.Println("salesforce was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The terminated employee was not in one of the selected cost centers at time of termination."
	}

	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("salesforce DELETE getAccessToken err: ", err)

		return HF_FAILED_LABEL, "Could not authenticate with Salesforce, please reconnect."
	}

	orgURL, ok := thisCompanyIntegration.Settings["api_url"].(string)
	if !ok {
		ErrorLog.Println("salesforce orgURL doesnt exist")

		return HF_FAILED_LABEL, "Could not authenticate with Salesforce, please reconnect."
	}

	sfQueryStr := `SELECT+Id+FROM+User+WHERE+Username='` + newFiredEvent.Email + `'`
	sfResponse, err := sfQuery(accessToken, orgURL, sfQueryStr)
	if err != nil {
		ErrorLog.Println("salesforce sfQuery doesnt exist: ", err)

		return HF_FAILED_LABEL, "Could not authenticate with Salesforce, please reconnect."
	}

	sfUserID := ""
	ok = false
	if len(sfResponse.Records) > 0 {
		user := sfResponse.Records[0]
		sfUserID, ok = user["Id"].(string)
		if !ok {
			ErrorLog.Println("salesforce cant find a user")

			return HF_FAILED_LABEL, "Could not find a User in your Salesforce org with that email address."
		}
	} else {
		ErrorLog.Println("salesforce cant find a user")

		return HF_FAILED_LABEL, "Could not find a User in your Salesforce org with that email address."
	}

	err = sfInactivateUser(accessToken, orgURL, sfUserID)
	if err != nil {
		ErrorLog.Println("sfFired delete err: ", err)

		return HF_FAILED_LABEL, err.Error()
	}

	InfoLog.Println("fired Salesforce user: ", newFiredEvent.Email, ", successfully removed from Salesforce")

	return HF_SUCCESS_LABEL, "Deactivated " + newFiredEvent.Email + " from Salesforce. They are no longer able to login."
}

func sfCreateUser(accessToken, orgURL string, sfUserReq SFCreateUserRequest) (HFCreatedCredentials, error) {
	newCredentials := HFCreatedCredentials{}

	urlStr := orgURL + "/services/data/v41.0/sobjects/User/"

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(sfUserReq)

	req, err := http.NewRequest("POST", urlStr, b)
	if err != nil {
		ErrorLog.Println("test new user NewRequest err: ", err)
		return newCredentials, errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("sf new user Do err: ", err)
		return newCredentials, errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		sfErrResponse := SFErrorResponse{}
		err = json.NewDecoder(resp.Body).Decode(&sfErrResponse)
		if err != nil {
			ErrorLog.Println("sfErrResponse error NewDecoder err: ", err)
			return newCredentials, errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
		}

		if len(sfErrResponse) > 0 {
			sfErr := sfErrResponse[0]
			ErrorLog.Println("sfErrResponse error len(sfErrResponse) > 0 err: ", sfErr)
			return newCredentials, errors.New(fmt.Sprintf("An error ocurred while adding to Salesforce: %s", sfErr.Message))
		} else {
			ErrorLog.Println("sf insert user ERROR: ", sfErrResponse)
			return newCredentials, errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
		}
	}

	sfResponse := SFCreateUserResponse{}
	err = json.NewDecoder(resp.Body).Decode(&sfResponse)
	if err != nil {
		ErrorLog.Println("sfCreateUser NewDecoder err: ", err)
		return newCredentials, errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	if sfResponse.ID == "" {
		ErrorLog.Println("sfCreateUser id response was blank: ")
		return newCredentials, errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	err = sfResetPassword(accessToken, orgURL, sfResponse.ID, passwords.EMAIL_TEMP_PASSWORD)
	if err != nil {
		// TODO: if this fails, the user was still created, somehow alert client
		ErrorLog.Println("sfCreateUser sfResetPassword err: ", err)
		// return errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	newCredentials.Password = passwords.EMAIL_TEMP_PASSWORD
	newCredentials.Username = sfUserReq.Username

	return newCredentials, nil
}

func sfInactivateUser(accessToken, orgURL, sfID string) error {
	urlStr := orgURL + "/services/data/v41.0/sobjects/User/" + sfID

	reqBod := map[string]interface{}{"IsActive": false}

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(reqBod)

	req, err := http.NewRequest("PATCH", urlStr, b)
	if err != nil {
		ErrorLog.Println("sfInactivateUser new user NewRequest err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("sf sfInactivateUser user Do err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("sf sfInactivateUser user ERROR: ", bodyString)
		return errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	return nil
}

func getSFName(accessToken string, thisCompanyIntegration CompanyIntegration) (string, error) {
	userInfoURL, ok := thisCompanyIntegration.Settings["sf_id"].(string)
	if !ok {
		ErrorLog.Println("sf getSFname but no userInfoURL!")
		return "", errors.New("An error occured.")
	}

	req, err := http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		ErrorLog.Println("getSFName NewRequest err: ", err)
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("getSFName Do err: ", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("getSFName err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return "", errors.New("bad request")
	}

	sfResponse := SFUserInfoResponse{}
	err = json.NewDecoder(resp.Body).Decode(&sfResponse)
	if err != nil {
		ErrorLog.Println("getSFName NewDecoder err: ", err)
		return "", errors.New("bad request")
	}

	return sfResponse.Email, nil
}

func constructNewSalesforceUser(accessToken, orgURL string, opCompanyID int64, opEmployeeRecord opapi.OPFullEmployeeRecord, thisCompanyIntegration CompanyIntegration, resultsSoFar []HFCompanyIntegrationResultJoin) (SFCreateUserRequest, error) {
	sfUserReq := SFCreateUserRequest{}

	sfQueryStr := `SELECT+Id,+FirstName,+ProfileId,+TimeZoneSidKey+FROM+User+WHERE+Profile.Name='Standard%20User'+LIMIT+1`
	sfResponse, err := sfQuery(accessToken, orgURL, sfQueryStr)
	if err != nil {
		ErrorLog.Println(err)
		return sfUserReq, errors.New("Salesforce reference User not found, cannot create new user")
	}

	timeZoneSidKey, profileId := "America/Los_Angeles", "xxx"
	if len(sfResponse.Records) > 0 {
		user := sfResponse.Records[0]

		if opEmployeeRecord.Timezone != "" {
			InfoLog.Println("setting timezone in salesforce from OP! tz: ", opEmployeeRecord.Timezone)
			timeZoneSidKey = opEmployeeRecord.Timezone
		} else {
			tzsk, ok := user["TimeZoneSidKey"].(string)
			if ok {
				timeZoneSidKey = tzsk
			}
		}

		pi, ok := user["ProfileId"].(string)
		if !ok {
			ErrorLog.Println("Salesforce reference User ProfileId not found, qutting!")
			return sfUserReq, errors.New("Salesforce reference User not found, cannot create new user.")
		} else {
			profileId = pi
		}
	} else {
		ErrorLog.Println("Salesforce reference User not found, looking up profile ID only")
		sfQueryStr = `SELECT+Id+FROM+Profile+WHERE+Name='Standard%20User'`
		sfResponse, err = sfQuery(accessToken, orgURL, sfQueryStr)
		if err != nil {
			ErrorLog.Println(err)
			return sfUserReq, errors.New("Salesforce Profile not found, cannot create new user.")
		}

		if len(sfResponse.Records) > 0 {
			profile := sfResponse.Records[0]

			pi2, ok := profile["Id"].(string)
			if !ok {
				ErrorLog.Println("Salesforce reference ProfileId not found, qutting!")
				return sfUserReq, errors.New("Salesforce Profile not found, cannot create new user.")
			} else {
				profileId = pi2
			}
		} else {
			ErrorLog.Println("Salesforce reference ProfileId not found, qutting!")
			return sfUserReq, errors.New("Salesforce Profile not found, cannot create new user.")
		}
	}

	alias := ""
	firstNameLower := strings.ToLower(opEmployeeRecord.FirstName)
	lastNameLower := strings.ToLower(opEmployeeRecord.LastName)
	if firstNameLower != "" {
		alias = string(firstNameLower[0])
		if len(lastNameLower) > 7 {
			alias = alias + lastNameLower[:7]
		} else {
			alias = alias + lastNameLower
		}
	} else {
		if len(lastNameLower) > 8 {
			alias = lastNameLower[:8]
		} else {
			alias = lastNameLower
		}
	}

	cxn := chooseOPAPICxn(opCompanyID)
	opJobTitle, err := cxn.GetOPJobTitle(opCompanyID, int64(opEmployeeRecord.ID))
	if err != nil {
		ErrorLog.Println("err getting OP job title: ", err)
	}

	company, _ := lookupCompanyByOPID(opCompanyID)

	emailToUse, err := generateEmailAddressBasedOnSettings(opEmployeeRecord, thisCompanyIntegration, resultsSoFar)
	if err != nil {
		ErrorLog.Println("generateEmailAddressBasedOnSettings errd: ", err.Error())

		return sfUserReq, errors.New(`Could not add to Salesforce: ` + err.Error())
	}

	sfUserReq = SFCreateUserRequest{
		Email:             emailToUse,
		FirstName:         opEmployeeRecord.FirstName,
		LastName:          opEmployeeRecord.LastName,
		Username:          emailToUse,     // unique in all of SF, format of email.
		Alias:             alias,          // Short name to identify the user on list pages, reports, and other pages where the entire name does not fit. Up to 8 characters are allowed in this field.
		TimeZoneSidKey:    timeZoneSidKey, // fetch?
		ProfileId:         profileId,      // fetch? or use OP timezone
		LanguageLocaleKey: "en_US",
		LocaleSidKey:      "en_US",
		EmailEncodingKey:  "UTF-8",
		IsActive:          true,
		City:              opEmployeeRecord.Address.City,
		Country:           opEmployeeRecord.Address.Country,
		State:             opEmployeeRecord.Address.State,
		Street:            opEmployeeRecord.Address.AddressLine1,
		EmployeeNumber:    opEmployeeRecord.EmployeeID,
		Title:             opJobTitle,
		CompanyName:       company.Name,
	}

	return sfUserReq, nil
}

func sfResetPassword(accessToken, orgURL, sfUserID, newPassword string) error {
	urlStr := orgURL + "/services/data/v41.0/sobjects/User/" + sfUserID + "/password"

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(map[string]interface{}{"NewPassword": newPassword})

	req, err := http.NewRequest("POST", urlStr, b)
	if err != nil {
		ErrorLog.Println("sfResetPassword new user NewRequest err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("sf new user Do err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("sf sfResetPassword user ERROR: ", bodyString)
		return errors.New("An error ocurred while adding to Salesforce. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	return nil
}

func sfQuery(accessToken, orgURL, query string) (SFQueryResponse, error) {
	qURL := orgURL + "/services/data/v41.0/query/?q=" + query
	sfResponse := SFQueryResponse{}

	req, err := http.NewRequest("GET", qURL, nil)
	if err != nil {
		ErrorLog.Println("sfGetUserIDByEmail NewRequest err: ", err)
		return sfResponse, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("sfGetUserIDByEmail Do err: ", err)
		return sfResponse, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("sfGetUserIDByEmail err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return sfResponse, errors.New("bad request")
	}

	err = json.NewDecoder(resp.Body).Decode(&sfResponse)
	if err != nil {
		ErrorLog.Println("sfGetUserIDByEmail NewDecoder err: ", err)
		return sfResponse, errors.New("bad request")
	}

	return sfResponse, nil
}
