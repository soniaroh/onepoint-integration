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

func boxHired(newHiredEvent *HFEvent, opCompanyID int64, fullEmployeeRecord opapi.OPFullEmployeeRecord, resultsSoFar []HFCompanyIntegrationResultJoin) (string, string, HFCreatedCredentials) {
	var createdCredentials = HFCreatedCredentials{}

	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(boxIntegrationsURL, newHiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("boxHired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}

	*newHiredEvent.Processed = true
	dbmap.Update(newHiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new hired but box disabled")

		return HF_DISABLED_LABEL, "Box Integration was disabled at the time of processing event.", createdCredentials
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new hired but not connected to Box")

		return HF_FAILED_LABEL, "Box is not connected.", createdCredentials
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("box checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue.", createdCredentials
	}
	if blockedByCC {
		ErrorLog.Println("box was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The hired employee was not in one of the selected cost centers at time of hire.", createdCredentials
	}

	createdCredentials, err = boxAddUser(thisCompanyIntegration, opCompanyID, fullEmployeeRecord, resultsSoFar)
	if err != nil {
		ErrorLog.Println("boxAddUser err: ", err)

		return HF_FAILED_LABEL, err.Error(), createdCredentials
	}

	InfoLog.Println("add box user: ", createdCredentials.Username, ", successfully added to box")

	return HF_SUCCESS_LABEL, fmt.Sprintf("Added %s to Box.", createdCredentials.Username), createdCredentials
}

func boxAddUser(thisCompanyIntegration CompanyIntegration, opCompanyID int64, fullEmployeeRecord opapi.OPFullEmployeeRecord, resultsSoFar []HFCompanyIntegrationResultJoin) (HFCreatedCredentials, error) {
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

	cxn := chooseOPAPICxn(opCompanyID)
	opJobTitle, err := cxn.GetOPJobTitle(opCompanyID, int64(fullEmployeeRecord.ID))
	if err != nil {
		ErrorLog.Println("err getting OP job title: ", err)
	}

	phoneNumber := ""
	preferredPhone := fullEmployeeRecord.Phones.PreferredPhone
	switch preferredPhone {
	case "CELL":
		if fullEmployeeRecord.Phones.CellPhone != "" {
			phoneNumber = fullEmployeeRecord.Phones.CellPhone
		}
	case "HOME":
		if fullEmployeeRecord.Phones.HomePhone != "" {
			phoneNumber = fullEmployeeRecord.Phones.HomePhone
		}
	case "WORK":
		if fullEmployeeRecord.Phones.WorkPhone != "" {
			phoneNumber = fullEmployeeRecord.Phones.WorkPhone
		}
	default:
		if fullEmployeeRecord.Phones.WorkPhone != "" {
			phoneNumber = fullEmployeeRecord.Phones.WorkPhone
		} else if fullEmployeeRecord.Phones.CellPhone != "" {
			phoneNumber = fullEmployeeRecord.Phones.CellPhone
		} else if fullEmployeeRecord.Phones.HomePhone != "" {
			phoneNumber = fullEmployeeRecord.Phones.HomePhone
		}
	}

	thisNewUser := BoxCreateUserBody{
		Name:        fullEmployeeRecord.FirstName + " " + fullEmployeeRecord.LastName,
		Login:       emailToUse,
		JobTitle:    opJobTitle,
		Phone:       phoneNumber,
		Address:     fmt.Sprintf("%s %s,%s", fullEmployeeRecord.Address.AddressLine1, fullEmployeeRecord.Address.City, fullEmployeeRecord.Address.State),
		SpaceAmount: 1000000000,
	}

	urlStr := "https://api.box.com/2.0/users"
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
		ErrorLog.Println("box new user Do err: ", err)
		return newCredentials, errors.New("Something went wrong on our end, we will investigate the issue.")
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		boxResponse := BoxErrorResponse{}
		err = json.NewDecoder(resp.Body).Decode(&boxResponse)
		if err != nil {
			ErrorLog.Println("box err NewDecoder err: ", err)
			return newCredentials, errors.New("Creation failed for unknown reason, we will investigate this issue.")
		}

		if boxResponse.Status == 403 {
			ErrorLog.Printf("box 403 insert err: %+v\n", boxResponse)
			return newCredentials, errors.New("Creation failed. The user currently used to connect this Integration to Box no longer has the necessary permissions, please reconnect with another Box admin.")
		}

		ErrorLog.Printf("Box non 403 insert err: %+v\n", boxResponse)
		return newCredentials, errors.New("Creation failed. Error message from Box: " + boxResponse.Message)
	}

	newCredentials.Password = fmt.Sprintf("Link sent to %s", emailToUse)
	newCredentials.Username = emailToUse

	return newCredentials, nil
}

func boxFired(newFiredEvent *HFEvent, fullEmployeeRecord opapi.OPFullEmployeeRecord) (string, string) {
	thisCompanyIntegration, err := getCompanyIntegrationByIntegrationString(boxIntegrationsURL, newFiredEvent.CompanyID)
	if err != nil {
		ErrorLog.Println("boxFired couldnt look up company integration: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}

	*newFiredEvent.Processed = true
	dbmap.Update(newFiredEvent)

	if !checkIntegrationEnabled(thisCompanyIntegration) {
		InfoLog.Println("new fired but box disabled")

		return HF_DISABLED_LABEL, "Box Integration was disabled at the time of processing event."
	}

	if thisCompanyIntegration.Status != "connected" {
		ErrorLog.Println("new fired but not connected to Box")

		return HF_FAILED_LABEL, "Box is not connected."
	}

	if newFiredEvent.Email == "" {
		ErrorLog.Println("trying to remove Box user but theres no email address")

		return HF_FAILED_LABEL, "Could not deactivate from Box because employee was terminated without an email address."
	}

	blockedByCC, err := checkHFCostCenterBlocked(thisCompanyIntegration, fullEmployeeRecord)
	if err != nil {
		ErrorLog.Println("box checkHFCostCenterBlocked err: ", err)
		return HF_FAILED_LABEL, "An error occured on our end, we will investigate this issue."
	}
	if blockedByCC {
		ErrorLog.Println("box was blocked by cost center")

		return HF_CC_BLOCKED_LABEL, "The terminated employee was not in one of the selected cost centers at time of termination."
	}

	err = boxDeactivateUser(thisCompanyIntegration, newFiredEvent.Email)
	if err != nil {
		ErrorLog.Println("boxDeactivateUser err: ", err)

		return HF_FAILED_LABEL, err.Error()
	}

	InfoLog.Println("fired Box user: ", newFiredEvent.Email, ", successfully suspended on Box")

	return HF_SUCCESS_LABEL, "Deactivated " + newFiredEvent.Email + " from Box. They are no longer able to login."
}

func boxDeactivateUser(thisCompanyIntegration CompanyIntegration, emailOfUserToSuspend string) error {
	accessToken, err := getAccessToken(thisCompanyIntegration)
	if err != nil {
		ErrorLog.Println("getAccessToken err: ", err)
		return errors.New("Could not authenticate with Box, please reconnect to Box.")
	}

	suspendBody := map[string]string{"status": "inactive"}

	boxUserID, err := boxLookupIdByEmail(accessToken, emailOfUserToSuspend)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not find Box user with the %s.", emailOfUserToSuspend))
	}

	urlStr := fmt.Sprintf("https://api.box.com/2.0/users/%s", boxUserID)
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
		boxResponse := BoxErrorResponse{}
		err = json.NewDecoder(resp.Body).Decode(&boxResponse)
		if err != nil {
			ErrorLog.Println("Box error NewDecoder err: ", err)
			return errors.New("Deactivation failed for unknown reason, we will investigate this issue.")
		}

		if boxResponse.Status == 403 {
			ErrorLog.Printf("g suite 403 suspend error %+v\n", boxResponse)
			return errors.New("Box User deactivation failed. The user currently used to connect this Integration to Box no longer has the necessary permissions, please reconnect with another Box admin.")
		}

		ErrorLog.Printf("g suite nON 403 suspend error %+v\n", boxResponse)
		return errors.New("Deactivation failed. This most likely occurred because the email address used was not found in Box. Refer to the Common Error Causes in the Integration Documentation below for more info.")
	}

	return nil
}

func boxLookupIdByEmail(accessToken, emailAddress string) (string, error) {
	urlStr := fmt.Sprintf("https://api.box.com/2.0/users?fields=id,login&filter_term=%s", emailAddress)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		ErrorLog.Println("boxLookupIdByEmail NewRequest err: ", err)
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("boxLookupIdByEmail Do err: ", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("boxLookupIdByEmail err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return "", errors.New("bad request")
	}

	boxResponse := BoxUserSearchResponse{}
	err = json.NewDecoder(resp.Body).Decode(&boxResponse)
	if err != nil {
		ErrorLog.Println("boxLookupIdByEmail NewDecoder err: ", err)
		return "", errors.New("bad request")
	}

	for _, user := range boxResponse.Entries {
		if user.Login == emailAddress {
			return user.ID, nil
		}
	}

	ErrorLog.Printf("boxLookupIdByEmail user not found !!! %+v\n", boxResponse)
	return "", errors.New("bad request")
}

func boxCheckAccess(accessToken string) error {
	urlStr := "https://api.box.com/2.0/users/me?fields=role"

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		ErrorLog.Println("boxCheckAccess NewRequest err: ", err)
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("boxCheckAccess Do err: ", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("boxCheckAccess err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return errors.New("bad request")
	}

	boxResponse := BoxUser{}
	err = json.NewDecoder(resp.Body).Decode(&boxResponse)
	if err != nil {
		ErrorLog.Println("boxCheckAccess NewDecoder err: ", err)
		return errors.New("bad request")
	}

	if boxResponse.Role == "admin" || boxResponse.Role == "coadmin" {
		return nil
	} else {
		ErrorLog.Println("boxResponse.Role was user! no access")
		return errors.New("403")
	}
}

func getBoxName(accessToken string) (string, error) {
	urlStr := "https://api.box.com/2.0/users/me"

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		ErrorLog.Println("getBoxName NewRequest err: ", err)
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		ErrorLog.Println("getBoxName Do err: ", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 201 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		ErrorLog.Println("getBoxName err response status: ", resp.StatusCode)
		ErrorLog.Println(bodyString)
		return "", errors.New("bad request")
	}

	boxResponse := BoxUser{}
	err = json.NewDecoder(resp.Body).Decode(&boxResponse)
	if err != nil {
		ErrorLog.Println("getBoxName NewDecoder err: ", err)
		return "", errors.New("bad request")
	}

	if boxResponse.Login != "" {
		return boxResponse.Login, nil
	} else {
		ErrorLog.Println("getBoxName response had no email addresses in response ")
		return "", errors.New("bad request")
	}
}
