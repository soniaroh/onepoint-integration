package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"opapi"
)

func m365Hired(newHiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string, HFCreatedCredentials) {
	var createdCredentials = HFCreatedCredentials{}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(m365IntegrationsURL, newHiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("m365Hired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}

	*newHiredEvent.Processed = true
	dbmap.Update(newHiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new hired but m365 disabled")

		return HF_DISABLED_LABEL, "Office 365 Integration was disabled at the time of processing event.", createdCredentials
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new hired but not connected to m365")

		return HF_FAILED_LABEL, "Office 365 not connected.", createdCredentials
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("m365 checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}
	if blockedByCC {
		ErrorLog.Println("m365 was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The hired employee was not in one of the selected cost centers at time of hire.", createdCredentials
	}

	createdCredentials, err = m365AddUser(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("m365Hired err: ", err)

		return HF_FAILED_LABEL, err.Error(), createdCredentials
	}

	InfoLog.Println("add m365 user: ", createdCredentials.Username, ", successfully added to m365")

	opEmailDescription := ""
	err = sendNewEmailToOP(newHiredEvent.CompanyID, fullEmployeeRecord, createdCredentials.Username)
	if err != nil {
		ErrorLog.Println("sendNewEmailToOP err: ", err)
		opEmailDescription = "The new address could not be added to OnePoint."
	} else {
		opEmailDescription = "This new address was added as the employee's Primary Email in OnePoint."
	}

	return HF_SUCCESS_LABEL, fmt.Sprintf("Added %s to Office 365. %s", createdCredentials.Username, opEmailDescription), createdCredentials
}

func m365Fired(newFiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string) {
	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(m365IntegrationsURL, newFiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("m365Fired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}

	*newFiredEvent.Processed = true
	dbmap.Update(newFiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new fired but m365 disabled")

		return HF_DISABLED_LABEL, "Office 365 Integration was disabled at the time of processing event."
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new fired but not connected to m365")

		return HF_FAILED_LABEL, "Office 365 not connected."
	}

	if newFiredEvent.Email == "" {
		ErrorLog.Println("trying to remove m365 user but theres no email address")

		return HF_FAILED_LABEL, "Could not delete from Office 365 because employee was terminated without an email address."
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("m365 checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}
	if blockedByCC {
		ErrorLog.Println("m365 was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The terminated employee was not in one of the selected cost centers at time of termination."
	}

	err = m365RemoveUser(thisCompanyIntegration, newFiredEvent.Email)
	if err != nil {
		ErrorLog.Println("m365 delete err: ", err)

		return HF_FAILED_LABEL, err.Error()
	}

	InfoLog.Println("fired Office 365 user: ", newFiredEvent.Email, ", successfully removed from Office 365")

	return HF_SUCCESS_LABEL, "Deleted " + newFiredEvent.Email + " from Office 365. They are no longer able to login."
}

func m365AddUser(thisCompanyIntegration CompanyIntegration, fullEmployeeRecord opapi.OPFullEmployeeRecord) (HFCreatedCredentials, error) {
	newCredentials := HFCreatedCredentials{}

	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("m365AddUser getAccessToken err: ", err)
		return newCredentials, errors.New("Could not authenticate with Office 365, please reconnect.")
	}

	// DisplayName       : This is usually the combination of the user's first name, middle initial and last name. This property is required when a user is created and it cannot be cleared during updates.
	// MailNickname      : The mail alias for the user. This property must be specified when a user is created.
	// UserPrincipalName : The user principal name (UPN) of the user. The UPN is an Internet-style login name for the user based on the Internet standard RFC 822. By convention, this should map to the user's email name. The general format is alias@domain, where domain must be present in the tenant’s collection of verified domains. This property is required when a user is created. The verified domains for the tenant can be accessed from the verifiedDomains property of organization.

	emailToUse, err := generateEmailAddressBasedOnSettings(fullEmployeeRecord, thisCompanyIntegration, []HFCompanyIntegrationResultJoin{})
	if err != nil {
		ErrorLog.Println("generateEmailAddressBasedOnSettings errd: ", err.Error())

		return newCredentials, errors.New(`Could not add to Office 365: ` + err.Error())
	}

	emailSplitArr := strings.Split(emailToUse, "@")

	thisNewUser := M365NewUserRequest{
		AccountEnabled: true,
		DisplayName:    fullEmployeeRecord.FirstName + " " + fullEmployeeRecord.LastName,
		PasswordProfile: M365PasswordProfile{
			Password:                      passwords.EMAIL_TEMP_PASSWORD,
			ForceChangePasswordNextSignIn: true,
		},
		MailNickname:      emailSplitArr[0],
		UserPrincipalName: emailToUse, // end of reqired fields
		City:              fullEmployeeRecord.Address.City,
		Country:           fullEmployeeRecord.Address.Country,
		MobilePhone:       fullEmployeeRecord.Phones.CellPhone,
		PostalCode:        fullEmployeeRecord.Address.Zip,
		State:             fullEmployeeRecord.Address.State,
		StreetAddress:     fullEmployeeRecord.Address.AddressLine1,
		GivenName:         fullEmployeeRecord.FirstName,
		Surname:           fullEmployeeRecord.LastName,
	}

	urlStr := "https://graph.microsoft.com/v1.0/users"
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
		m365Response := M365ErrorResponseBody{}
		err = json.NewDecoder(resp.Body).Decode(&m365Response)
		if err != nil {
			ErrorLog.Println("m365Response NewDecoder err: ", err)
			return newCredentials, errors.New("Creation failed for unknown reason, we will investigate this issue.")
		}

		if m365Response.Error.Code == "Authorization_RequestDenied" {
			return newCredentials, errors.New("The Office 365 user that is currently used to connect this integration no longer has the access privileges required to perform this action. Please reconnect with another Office 365 admin.")
		}

		ErrorLog.Println("m365 insert user ERROR response msg: ", m365Response.Error.Message)
		return newCredentials, errors.New("An error ocurred while adding to Office 365. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	newCredentials.Password = passwords.EMAIL_TEMP_PASSWORD
	newCredentials.Username = emailToUse

	return newCredentials, nil
}

func m365RemoveUser(thisCompanyIntegration CompanyIntegration, emailAddressToDelete string) error {
	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return errors.New("Could not authenticate, please reconnect to Office 365.")
	}

	urlStr := "https://graph.microsoft.com/v1.0/users/" + emailAddressToDelete

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
		m365Response := M365ErrorResponseBody{}
		err = json.NewDecoder(resp.Body).Decode(&m365Response)
		if err != nil {
			ErrorLog.Println("m365Response error NewDecoder err: ", err)
			return errors.New("Deletion failed for unknown reason, we will investigate this issue.")
		}

		ErrorLog.Println("Office 365 delete user ERROR response msg: ", m365Response.Error.Message, " CODE: ", m365Response.Error.Code)

		if m365Response.Error.Code == "Authorization_RequestDenied" {
			return errors.New("The Office 365 user that is currently used to connect this integration no longer has the access privileges required to perform this action. Please reconnect with another Office 365 admin.")
		}

		return errors.New("Deletion failed. Please refer to the Common Error Causes in the Integration Documentation below.")
	}

	return nil
}

func getM365Name(accessToken string) (string, error) {
	urlStr := "https://graph.microsoft.com/v1.0/me"

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		ErrorLog.Println("getM365Name NewRequest err: ", err)
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("getM365Name Do err: ", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("getM365Name err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return "", errors.New("bad request")
	}

	m365Response := M365UserResource{}
	err = json.NewDecoder(resp.Body).Decode(&m365Response)
	if err != nil {
		ErrorLog.Println("google NewDecoder err: ", err)
		return "", errors.New("bad request")
	}

	if m365Response.Mail != "" {
		return m365Response.Mail, nil
	} else {
		ErrorLog.Println("getM365Name response had no email addresses in response ")
		return "", errors.New("bad request")
	}
}

func m365TestDeleteUser() {
	email := "test@onepointhcm2.onmicrosoft.com"
	companyID := 1

	compInt, err := getCompanyIntegrationByIntegrationString("microsoft365", int64(companyID))
	if err != nil {
		ErrorLog.Println(err)
		return
	}

	err = m365RemoveUser(compInt, email)

	if err != nil {
		ErrorLog.Println(err)
		return
	}
}

// func m365ListLicenses() {
// 	companyID := 1
//
// 	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString("microsoft365", int64(companyID))
// 	if err != nil {
// 		ErrorLog.Println("m365ListLicenses couldnt look up company integration: ", err)
// 		return
// 	}
//
// 	accessToken, err := getAccessToken(thisCompanyIntegration)
// 	if err != nil {
// 		ErrorLog.Println("getAccessToken err: ", err)
// 		return
// 	}
//
// 	urlStr := "https://graph.microsoft.com/v1.0/subscribedSkus"
//
// 	req, err := http.NewRequest("GET", urlStr, nil)
// 	if err != nil {
// 		ErrorLog.Println("m365ListLicenses NewRequest err: ", err)
// 		return
// 	}
// 	req.Header.Set("Accept", "application/json")
// 	req.Header.Set("Authorization", "Bearer "+accessToken)
//
// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil {
// 		ErrorLog.Println("m365ListLicenses Do err: ", err)
// 		return
// 	}
// 	defer resp.Body.Close()
//
// 	if resp.StatusCode > 201 {
// 		bodyBytes, _ := ioutil.ReadAll(resp.Body)
// 		bodyString := string(bodyBytes)
//
// 		ErrorLog.Println("m365ListLicenses err response status: ", resp.StatusCode)
// 		ErrorLog.Println(bodyString)
// 		return
// 	}
//
// 	m365Response := M365SkusResponse{}
// 	err = json.NewDecoder(resp.Body).Decode(&m365Response)
// 	if err != nil {
// 		ErrorLog.Println("google NewDecoder err: ", err)
// 		return
// 	}
//
// 	spew.Dump(m365Response)
// }

func m365HiredTest() {
	accountID := 48668169
	opCompanyID := 33560858
	companyID := 1

	fname := "igo"
	lname := "test"
	email := "igortest@onepointhcm.com"

	falseV := false
	newHiredEvent := HFEvent{
		Type:        HIRED_TYPE_ID,
		OPAccountID: int64(accountID),
		EventDate:   "03/12/2018",
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

	m365Hired(&newHiredEvent, employeeRecord)
}
