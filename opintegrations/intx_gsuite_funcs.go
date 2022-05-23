package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"opapi"
)

const (
	EMAIL_PATTERN_FROM_OP       = "use_op"
	EMAIL_PATTERN_ADJACENT      = "adjacent"
	EMAIL_PATTERN_PERIOD        = "period"
	EMAIL_PATTERN_FIRST_INITIAL = "first_initial"
	EMAIL_PATTERN_OP_USERNAME   = "op_username"
)

func gSuiteHired(newHiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string, HFCreatedCredentials) {
	var createdCredentials = HFCreatedCredentials{}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(gSuiteIntegrationsURL, newHiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("gSuiteHired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}

	*newHiredEvent.Processed = true
	dbmap.Update(newHiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new hired but gsuite disabled")

		return HF_DISABLED_LABEL, "G Suite Integration was disabled at the time of processing event.", createdCredentials
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new hired but not connected to gsuite")

		return HF_FAILED_LABEL, "G Suite is not connected.", createdCredentials
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("gsuite checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}
	if blockedByCC {
		ErrorLog.Println("gsuite was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The hired employee was not in one of the selected cost centers at time of hire.", createdCredentials
	}

	createdCredentials, err = gSuiteAddUser(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("gSuiteAddUser err: ", err)

		return HF_FAILED_LABEL, err.Error(), createdCredentials
	}

	InfoLog.Println("add g suite user: ", createdCredentials.Username, ", successfully added to g suite")

	opEmailDescription := ""
	err = sendNewEmailToOP(newHiredEvent.CompanyID, fullEmployeeRecord, createdCredentials.Username)
	if err != nil {
		ErrorLog.Println("sendNewEmailToOP err: ", err)
		opEmailDescription = "The new address could not be added to OnePoint."
	} else {
		opEmailDescription = "This new address was added as the employee's Primary Email in OnePoint."
	}

	return HF_SUCCESS_LABEL, fmt.Sprintf("Added %s to G Suite. %s", createdCredentials.Username, opEmailDescription), createdCredentials
}

func gSuiteFired(newFiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string) {
	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(gSuiteIntegrationsURL, newFiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("gSuiteFired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}

	*newFiredEvent.Processed = true
	dbmap.Update(newFiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new fired but gsuite disabled")

		return HF_DISABLED_LABEL, "G Suite Integration was disabled at the time of processing event."
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new fired but not connected to gsuite")

		return HF_FAILED_LABEL, "G Suite is not connected."
	}

	if newFiredEvent.Email == "" {
		ErrorLog.Println("trying to remove gsuite user but theres no email address")

		return HF_FAILED_LABEL, "Could not suspend from G Suite because employee was terminated without an email address."
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("gsuite checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}
	if blockedByCC {
		ErrorLog.Println("gsuite was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The terminated employee was not in one of the selected cost centers at time of termination."
	}

	err = gSuiteSuspendUser(thisCompanyIntegration, newFiredEvent.Email)
	if err != nil {
		ErrorLog.Println("gSuiteDeleteUser err: ", err)

		return HF_FAILED_LABEL, err.Error()
	}

	InfoLog.Println("fired g suite user: ", newFiredEvent.Email, ", successfully suspended on g suite")

	return HF_SUCCESS_LABEL, "Suspended " + newFiredEvent.Email + " from G Suite. They are no longer able to login."
}

func gSuiteDeleteUser(thisCompanyIntegration CompanyIntegration, emailOfUserToDelete string) error {
	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return errors.New("Could not authenticate with G Suite, please reconnect to G Suite.")
	}

	urlStr := "https://www.googleapis.com/admin/directory/v1/users/" + emailOfUserToDelete

	req, err := http.NewRequest("DELETE", urlStr, nil)
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

	if resp.StatusCode > 300 {
		gSuiteResponse := GSuiteErrorResponseBody{}
		err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
		if err != nil {
			ErrorLog.Println("google error NewDecoder err: ", err)
			return errors.New("Deletion failed for unknown reason, we will investigate this issue.")
		}

		if gSuiteResponse.Error.Code == 403 {
			ErrorLog.Printf("g suite 403 delete error %+v\n", gSuiteResponse)
			return errors.New("G Suite User deletion failed. The user currently used to connect this Integration to G Suite no longer has the necessary permissions, please reconnect with another G Suite admin.")
		}

		ErrorLog.Println("G suite non 404 delete user ERROR response msg: ", gSuiteResponse.Error.Message)
		return errors.New("Deletion failed. This most likely occurred because the email address used was not found in G Suite. Refer to the Common Error Causes in the Integration Documentation below for more info.")
	}

	return nil
}

func gSuiteSuspendUser(thisCompanyIntegration CompanyIntegration, emailOfUserToSuspend string) error {
	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return errors.New("Could not authenticate with G Suite, please reconnect to G Suite.")
	}

	suspendBody := map[string]bool{"suspended": true}

	urlStr := "https://www.googleapis.com/admin/directory/v1/users/" + emailOfUserToSuspend
	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(suspendBody)

	req, err := http.NewRequest("PUT", urlStr, b)
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

	if resp.StatusCode > 300 {
		gSuiteResponse := GSuiteErrorResponseBody{}
		err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
		if err != nil {
			ErrorLog.Println("google error NewDecoder err: ", err)
			return errors.New("Suspension failed for unknown reason, we will investigate this issue.")
		}

		if gSuiteResponse.Error.Code == 403 {
			ErrorLog.Printf("g suite 403 suspend error %+v\n", gSuiteResponse)
			return errors.New("G Suite User suspension failed. The user currently used to connect this Integration to G Suite no longer has the necessary permissions, please reconnect with another G Suite admin.")
		}

		ErrorLog.Printf("g suite nON 403 suspend error %+v\n", gSuiteResponse)
		return errors.New("Suspension failed. This most likely occurred because the email address used was not found in G Suite. Refer to the Common Error Causes in the Integration Documentation below for more info.")
	}

	return nil
}

func gSuiteAddUser(thisCompanyIntegration CompanyIntegration, fullEmployeeRecord opapi.OPFullEmployeeRecord) (HFCreatedCredentials, error) {
	newCredentials := HFCreatedCredentials{}

	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return newCredentials, errors.New("Could not authenticate with G Suite, please reconnect.")
	}

	// TODO: choose password
	passwordToUse := passwords.EMAIL_TEMP_PASSWORD

	thisNewUserName := GSuiteNameResource{
		GivenName:  fullEmployeeRecord.FirstName,
		FamilyName: fullEmployeeRecord.LastName,
	}

	zipCode := fullEmployeeRecord.Address.Zip
	if len(fullEmployeeRecord.Address.Zip) > 5 {
		zipCode = zipCode[:5]
	}

	address := GSuiteAddress{
		Locality:      fullEmployeeRecord.Address.City,
		State:         fullEmployeeRecord.Address.State,
		PostalCode:    zipCode,
		StreetAddress: fullEmployeeRecord.Address.AddressLine1,
		Type:          "home",
		CustomType:    "",
	}

	preferredPhone := fullEmployeeRecord.Phones.PreferredPhone

	phones := []GSuitePhone{}
	if fullEmployeeRecord.Phones.CellPhone != "" {
		gsp := GSuitePhone{
			Primary: preferredPhone == "CELL",
			Type:    "mobile",
			Value:   fullEmployeeRecord.Phones.CellPhone,
		}
		phones = append(phones, gsp)
	}

	if fullEmployeeRecord.Phones.HomePhone != "" {
		gsp := GSuitePhone{
			Primary: preferredPhone == "HOME",
			Type:    "home",
			Value:   fullEmployeeRecord.Phones.HomePhone,
		}
		phones = append(phones, gsp)
	}

	if fullEmployeeRecord.Phones.WorkPhone != "" {
		gsp := GSuitePhone{
			Primary: preferredPhone == "WORK",
			Type:    "work",
			Value:   fullEmployeeRecord.Phones.WorkPhone,
		}
		phones = append(phones, gsp)
	}

	emailToUse, err := generateEmailAddressBasedOnSettings(fullEmployeeRecord, thisCompanyIntegration, []HFCompanyIntegrationResultJoin{})
	if err != nil {
		ErrorLog.Println("generateEmailAddressBasedOnSettings errd: ", err.Error())

		return newCredentials, errors.New(`Could not add to G Suite: ` + err.Error())
	}

	thisNewUser := GSuiteCreateUserBody{
		Name:                    thisNewUserName,
		Password:                passwordToUse,
		PrimaryEmail:            emailToUse,
		ChangePasswordNextLogin: true,
		Addresses:               []GSuiteAddress{address},
		Phones:                  phones,
	}

	urlStr := "https://www.googleapis.com/admin/directory/v1/users"
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
		ErrorLog.Println("est new user Do err: ", err)
		return newCredentials, errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		gSuiteResponse := GSuiteErrorResponseBody{}
		err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
		if err != nil {
			ErrorLog.Println("google NewDecoder err: ", err)
			return newCredentials, errors.New("Creation failed for unknown reason, we will investigate this issue.")
		}

		if gSuiteResponse.Error.Code == 403 {
			ErrorLog.Printf("g suite 403 insert err: %+v\n", gSuiteResponse)
			return newCredentials, errors.New("Creation failed. The user currently used to connect this Integration to G Suite no longer has the necessary permissions, please reconnect with another G Suite admin.")
		}

		ErrorLog.Printf("g suite non 403 insert err: %+v\n", gSuiteResponse)
		return newCredentials, errors.New("Creation failed. Error message from G Suite: " + gSuiteResponse.Error.Message)
	}

	newCredentials.Password = passwordToUse
	newCredentials.Username = emailToUse

	return newCredentials, nil
}

func getGSuiteUserName(accessToken string) (string, error) {
	urlStr := "https://www.googleapis.com/oauth2/v2/userinfo?fields=email"

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		ErrorLog.Println("getGSuiteUserName NewRequest err: ", err)
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("getGSuiteUserName Do err: ", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("getGSuiteUserName err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return "", errors.New("bad request")
	}

	gSuiteResponse := GSuiteGetUserEmailResponseV2{}
	err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
	if err != nil {
		ErrorLog.Println("google NewDecoder err: ", err)
		return "", errors.New("bad request")
	}

	if gSuiteResponse.Email != "" {
		return gSuiteResponse.Email, nil
	} else {
		ErrorLog.Println("getGSuiteUserName response had no email addresses in response ")
		return "", errors.New("bad request")
	}
}

func gSuiteTestAccess(emailAddress, accessToken string) error {
	urlStr := "https://www.googleapis.com/admin/directory/v1/users/" + emailAddress

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		ErrorLog.Println("gSuiteTest NewRequest err: ", err)
		return errors.New("gSuiteTest NewRequest err")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("gSuiteTest Do err: ", err)
		return errors.New("gSuiteTest Do err")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("gSuiteTest err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)

		if resp.StatusCode == 403 {
			ErrorLog.Println("deeming g suite user not authorized")
			return errors.New("403")
		}

		return errors.New("error!")
	}

	gSuiteResponse := GSuiteUserResource{}
	err = json.NewDecoder(resp.Body).Decode(&gSuiteResponse)
	if err != nil {
		ErrorLog.Println("gSuiteTest NewDecoder err: ", err)
		return errors.New("gSuiteTest NewDecoder err")
	}

	return nil
}

func gSuiteTest() {
	name, err := getGSuiteUserName("ya29.Glx-BSrt5yQSQVnG4q1R_iSV6ZwsEEQ_6_QUuwFfsF0paZjPrVbsgoiquj7b62kaqkpChN2S1GPemkRKluR-iBaGKIxEr_qzbkxAJc0lTgySCOQBztOy2P0J5dFffg")
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	InfoLog.Println(name)
}

func gSuiteHiredTest() {
	accountID := 48285718
	opCompanyID := 33560858
	companyID := 1

	fname := "test"
	lname := "test"
	email := "test263546@employernet.net"

	falseV := false
	newHiredEvent := HFEvent{
		Type:        HIRED_TYPE_ID,
		OPAccountID: int64(accountID),
		EventDate:   "03/13/2018",
		CompanyID:   int64(companyID),
		FirstName:   fname,
		LastName:    lname,
		Email:       email,
		Processed:   &falseV,
		Created:     time.Now().Unix(),
	}

	dbmap.Insert(&newHiredEvent)

	cxn := chooseOPAPICxn(int64(opCompanyID))
	employeeRecord, err := cxn.GetFullEmployeeInfo(int64(accountID), int64(opCompanyID))
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	gSuiteFired(&newHiredEvent, employeeRecord)
}
