package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"opapi"
)

func dropboxHired(newHiredEvent *HFEvent, opCompanyID int64, fullEmployeeRecord opapi.OPFullEmployeeRecord, resultsSoFar []HFCompanyIntegrationResultJoin) (string, string, HFCreatedCredentials) {
	var createdCredentials = HFCreatedCredentials{}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(dropboxIntegrationsURL, newHiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("dropboxHired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}

	*newHiredEvent.Processed = true
	dbmap.Update(newHiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new hired but box disabled")

		return HF_DISABLED_LABEL, "Dropbox Integration was disabled at the time of processing event.", createdCredentials
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new hired but not connected to Dropbox")

		return HF_FAILED_LABEL, "Dropbox is not connected.", createdCredentials
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("dropbox checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}
	if blockedByCC {
		ErrorLog.Println("dropbox was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The hired employee was not in one of the selected cost centers at time of hire.", createdCredentials
	}

	createdCredentials, err = dropboxAddUser(thisCompanyIntegration, opCompanyID, fullEmployeeRecord, resultsSoFar)
	if err != nil {
		ErrorLog.Println("dropboxAddUser err: ", err)

		return HF_FAILED_LABEL, err.Error(), createdCredentials
	}

	InfoLog.Println("add Dropbox user: ", createdCredentials.Username, ", successfully added to Dropbox")

	return HF_SUCCESS_LABEL, fmt.Sprintf("Added %s to Dropbox.", createdCredentials.Username), createdCredentials
}

func dropboxAddUser(thisCompanyIntegration CompanyIntegration, opCompanyID int64, fullEmployeeRecord opapi.OPFullEmployeeRecord, resultsSoFar []HFCompanyIntegrationResultJoin) (HFCreatedCredentials, error) {
	newCredentials := HFCreatedCredentials{}

	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return newCredentials, errors.New("Could not authenticate with Box, please reconnect.")
	}

	emailToUse, err := generateEmailAddressBasedOnSettings(fullEmployeeRecord, thisCompanyIntegration, resultsSoFar)
	if err != nil {
		ErrorLog.Println("generateEmailAddressBasedOnSettings errd: ", err.Error())

		return newCredentials, errors.New(`Could not add to Box: ` + err.Error())
	}

	newMembers := []DropboxNewUser{
		DropboxNewUser{
			MemberEmail:      emailToUse,
			MemberGivenName:  fullEmployeeRecord.FirstName,
			MemberSurname:    fullEmployeeRecord.LastName,
			MemberExternalID: fullEmployeeRecord.EmployeeID,
			SendWelcomeEmail: true,
			Role:             "member_only",
		},
	}

	thisNewUser := DropboxNewUserRequestBody{
		ForceAsync: false,
		NewMembers: newMembers,
	}

	urlStr := "https://api.dropboxapi.com/2/team/members/add"
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(thisNewUser)

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
		ErrorLog.Println("dropbox new user Do err: ", err)
		return newCredentials, errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		if resp.StatusCode == 403 {
			ErrorLog.Printf("dropbox 403 insert err: %+s\n", bodyString)
			return newCredentials, errors.New("Creation failed. The user currently used to connect this Integration to Dropbox no longer has the necessary permissions, please reconnect with another Dropbox admin.")
		}

		ErrorLog.Printf("Dropbox non 403 insert err: %+s\n", bodyString)
		return newCredentials, errors.New("Creation failed. We will invistigate the cause of this issue.")
	}

	newCredentials.Password = fmt.Sprintf("Link sent to %s", emailToUse)
	newCredentials.Username = emailToUse

	return newCredentials, nil
}

func dropboxFired(newFiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string) {
	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(dropboxIntegrationsURL, newFiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("dropboxFired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}

	*newFiredEvent.Processed = true
	dbmap.Update(newFiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new fired but dropbox disabled")

		return HF_DISABLED_LABEL, "Dropbox Integration was disabled at the time of processing event."
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new fired but not connected to Dropbox")

		return HF_FAILED_LABEL, "Dropbox is not connected."
	}

	if newFiredEvent.Email == "" {
		ErrorLog.Println("trying to remove Dropbox user but theres no email address")

		return HF_FAILED_LABEL, "Could not suspend from Dropbox because employee was terminated without an email address."
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("Dropbox checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}
	if blockedByCC {
		ErrorLog.Println("Dropbox was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The terminated employee was not in one of the selected cost centers at time of termination."
	}

	err = dropboxSuspendUser(thisCompanyIntegration, newFiredEvent.Email)
	if err != nil {
		ErrorLog.Println("DropboxDeactivateUser err: ", err)

		return HF_FAILED_LABEL, err.Error()
	}

	InfoLog.Println("fired Dropbox user: ", newFiredEvent.Email, ", successfully suspended on Dropbox")

	return HF_SUCCESS_LABEL, "Suspended " + newFiredEvent.Email + " from Dropbox. They are no longer able to login."
}

func dropboxSuspendUser(thisCompanyIntegration CompanyIntegration, emailOfUserToSuspend string) error {
	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return errors.New("Could not authenticate with Box, please reconnect to Box.")
	}

	suspendedUser := map[string]interface{}{
		".tag":  "email",
		"email": emailOfUserToSuspend,
	}

	suspendBody := DropboxSuspendUserRequestBody{WipeData: true, User: suspendedUser}

	url := "https://api.dropboxapi.com/2/team/members/suspend"

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(suspendBody)

	req, err := http.NewRequest("POST", url, b)
	if err != nil {
		ErrorLog.Println("test new user NewRequest err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("est new user Do err: ", err)
		return errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		dropboxResponse := DropboxSuspendUserError{}
		err = json.NewDecoder(resp.Body).Decode(&dropboxResponse)
		if err != nil {
			ErrorLog.Println("Dropbox error NewDecoder err: ", err)
			return errors.New("Suspension failed for unknown reason, we will investigate this issue.")
		}

		errLabel := "This most likely occurred because the email address used was not found in Dropbox. Refer to the Common Error Causes in the Integration Documentation below for more info."
		switch dropboxResponse.Error.Tag {
		case "user_not_found", "user_not_in_team":
			errLabel = fmt.Sprintf("%s was not found in your Dropbox team.", emailOfUserToSuspend)
		case "suspend_inactive_user":
			errLabel = fmt.Sprintf("%s is not active, so they cannot be suspended.", emailOfUserToSuspend)
		case "suspend_last_admin":
			errLabel = fmt.Sprintf("%s is the last admin of the team, so they cannot be suspended.", emailOfUserToSuspend)
		case "team_license_limit":
			errLabel = "Team is full. The organization has no available licenses."
		}

		ErrorLog.Printf("Dropbox nON 403 suspend error %+v\n", dropboxResponse)
		return errors.New(fmt.Sprintf("Suspension failed. %s", errLabel))
	}

	return nil
}

func getDropboxName(accessToken string) (string, error) {
	username, err := dropboxGetLoggedInUser(accessToken)
	return username, err
}

func dropboxCheckAccess(emailAddress string) error {
	if emailAddress != "" {
		return nil
	}

	return errors.New("if getDropboxName failed then not admin")
}

// will return err if not admin, so we can use for both username and checkAccess
func dropboxGetLoggedInUser(accessToken string) (string, error) {
	urlStr := "https://api.dropboxapi.com/2/team/token/get_authenticated_admin"

	req, err := http.NewRequest("POST", urlStr, nil)
	if err != nil {
		ErrorLog.Println("dropboxGetLoggedInUser NewRequest err: ", err)
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("dropboxGetLoggedInUser Do err: ", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 200 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("dropboxGetLoggedInUser err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return "", errors.New("bad request")
	}

	dbResp := DropboxGetAdminUserResponse{}
	err = json.NewDecoder(resp.Body).Decode(&dbResp)
	if err != nil {
		ErrorLog.Println("dropboxGetLoggedInUser NewDecoder err: ", err)
		return "", errors.New("bad request")
	}

	if dbResp.AdminProfile.Email != "" {
		return dbResp.AdminProfile.Email, nil
	}

	ErrorLog.Printf("dropboxGetLoggedInUser got to end with no email: %+v\n", dbResp)

	return "", errors.New("bad request")
}
