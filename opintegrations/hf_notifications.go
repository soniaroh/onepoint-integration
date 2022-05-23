package main

import (
	"errors"
	"fmt"
	"html/template"

	"opapi"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

func sendHFSummaryEmail(productSettings map[string]interface{}, fullEmployeeRecord opapi.OPFullEmployeeRecord, newHFEvent *HFEvent, allResults []HFCompanyIntegrationResultJoin) {
	emailBody := HFSummaryEmailBody{
		HiredTrueFiredFalse: newHFEvent.Type == HIRED_TYPE_ID,
		EmployeeName:        fmt.Sprintf("%s %s", fullEmployeeRecord.FirstName, fullEmployeeRecord.LastName),
	}

	alertableCounter := 0

	for _, resultCombo := range allResults {
		if resultCombo.Result.Status == HF_FAILED_LABEL {
			errS := struct {
				Name        string
				Description string
			}{
				Name:        resultCombo.CompanyIntegration.IntegrationName,
				Description: resultCombo.Result.Description,
			}
			emailBody.ErrdServices = append(emailBody.ErrdServices, errS)
			alertableCounter++
		} else if resultCombo.Result.Status == HF_SUCCESS_LABEL {
			if newHFEvent.Type == HIRED_TYPE_ID {
				emailBody.HiredResults = append(emailBody.HiredResults, HiredSummaryEmailEvent{resultCombo.CompanyIntegration.IntegrationName, resultCombo.GeneratedCredentials.Username, resultCombo.GeneratedCredentials.Password})
				alertableCounter++
			} else {
				emailBody.FiredResults = append(emailBody.FiredResults, FiredSummaryEmailEvent{resultCombo.CompanyIntegration.IntegrationName, resultCombo.Result.Description})
				alertableCounter++
			}
		}
	}

	subjectType := "HIRED"
	if newHFEvent.Type == FIRED_TYPE_ID {
		subjectType = "TERMINATED"
	}

	toStringArr, err := getEmployeesToEmailFromSetting(productSettings, newHFEvent)
	if err != nil {
		ErrorLog.Println(err)
	}

	tos := []*sgmail.Email{}
	for _, toStr := range toStringArr {
		tos = append(tos, &sgmail.Email{Address: toStr})
	}

	emailHeaderInfo := sgEmailFields{
		Subject: fmt.Sprintf("OnePoint User Provisioning: %s %s", emailBody.EmployeeName, subjectType),
		From:    &sgmail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      tos,
		Bcc:     []*sgmail.Email{&sgmail.Email{Address: passwords.ADMIN_NOTIFICATION_EMAIL_ADDRESS}},
	}

	if alertableCounter > 0 {
		err := sendTemplatedEmailSendGrid(emailHeaderInfo, HF_SUMMARY_EMAIL_TEMPLATE, emailBody, "user_provisioning_summary")
		if err != nil {
			ErrorLog.Printf("emailing err: %v\n", err)
		} else {
			InfoLog.Printf("summary email sent successfully: %+v\n", emailHeaderInfo)
		}
	}
}

func getEmployeesToEmailFromSetting(productSettings map[string]interface{}, newHFEvent *HFEvent) ([]string, error) {
	toArr := []string{}

	if settingInterface, ok := productSettings["notification_email_recipient"]; ok {
		settingArr, ok := settingInterface.([]interface{})
		if ok {
			for _, setting := range settingArr {
				settingAsted, ok := setting.(map[string]interface{})
				if ok {
					typex, ok := settingAsted["type"].(string)
					if ok {
						if typex == "email" {
							address, ok := settingAsted["address"].(string)
							if ok {
								toArr = append(toArr, address)
								continue
							} else {
								ErrorLog.Println("type === email but cannot find address")
								continue
							}
						}
					}

					accountIDToSendTo, ok := settingAsted["id"].(float64)
					if ok {
						opCoID, err := getOPCompanyID(newHFEvent.CompanyID)
						if err != nil {
							return toArr, errors.New(fmt.Sprintf("cant lookup op co id! %+v  \n", err))
						} else {
							cxn := chooseOPAPICxn(opCoID)
							empRecord, err := cxn.GetFullEmployeeInfo(int64(accountIDToSendTo), opCoID)
							if err != nil {
								return toArr, errors.New(fmt.Sprintf("err AND we are sending! %+v  \n", err))
							} else {
								if empRecord.PrimaryEmail != "" {
									toArr = append(toArr, empRecord.PrimaryEmail)
								} else {
									return toArr, errors.New(fmt.Sprintf("setting employee has no primary email %+v  \n %+v  \n", empRecord, err))
								}
							}
						}
					} else {
						return toArr, errors.New(fmt.Sprintf("got settingArr but no ID found! %+v %+v \n", settingInterface, productSettings))
					}
				} else {
					return toArr, errors.New("settingAsted assertion failed")
				}
			}
		} else {
			return toArr, errors.New(fmt.Sprintf("notification_email_recipient but asserting err AND we are sending! %+v  \n %+v \n", settingInterface, productSettings))
		}
	} else {
		return toArr, errors.New(fmt.Sprintf("NO notification_email_recipient AND we are sending! %+v \n", productSettings))
	}

	return toArr, nil
}

type HFADSummaryEmailBody struct {
	Explanation template.HTML
}

func sendActiveDirectoryUserProvisioningEmail(event HFEvent, result HFResult, coInt CompanyIntegration, userCreated HFActionUser) error {
	successAddTemp := `
		<p style="margin-bottom: 10px;">
			%s was successfully added to Active Directory with temporary password of %s.
		</p>
	`
	successDeleteTemp := `
		<p style="margin-bottom: 10px;">
			%s was successfully disabled from Active Directory.
		</p>
	`

	failAddTemp := `
		<p style="margin-bottom: 10px;">
			An error occured while attempting to add %s to Active Directory.
		</p>
		<p style="margin-bottom: 10px;">
			Error message: %s
		</p>
	`

	failDeleteTemp := `
		<p style="margin-bottom: 10px;">
			An error occured while attempting to disable %s from Active Directory.
		</p>
		<p style="margin-bottom: 10px;">
			Error message: %s
		</p>
	`

	subjectType := ""
	text := ""
	employeeName := fmt.Sprintf("%s %s", event.FirstName, event.LastName)

	if event.Type == FIRED_TYPE_ID {
		subjectType = "TERMINATED"
		if result.Status == HF_FAILED_LABEL {
			text = fmt.Sprintf(failDeleteTemp, employeeName, result.Description)
		} else if result.Status == HF_SUCCESS_LABEL {
			text = fmt.Sprintf(successDeleteTemp, employeeName)
		} else {
			ErrorLog.Println("event result was not failed nor success")
			return errors.New("event result was not failed nor success")
		}
	} else if event.Type == HIRED_TYPE_ID {
		subjectType = "HIRED"
		if result.Status == HF_FAILED_LABEL {
			text = fmt.Sprintf(failAddTemp, employeeName, result.Description)
		} else if result.Status == HF_SUCCESS_LABEL {
			text = fmt.Sprintf(successAddTemp, employeeName, userCreated.Password)
		} else {
			ErrorLog.Println("event result was not failed nor success")
			return errors.New("event result was not failed nor success")
		}
	} else {
		ErrorLog.Printf("event.Type was neither: %d\n", event.Type)
		return errors.New(fmt.Sprintf("event.Type was neither: %d\n", event.Type))
	}

	emailBody := HFADSummaryEmailBody{
		Explanation: template.HTML(text),
	}

	coProd, err := getCompanyProductByURL(event.CompanyID, PRODUCT_USERPROVISIONING_URL)
	if err != nil {
		ErrorLog.Println("getCompanyProductByURL err: " + err.Error())
		return err
	}

	toStringArr, err := getEmployeesToEmailFromSetting(coProd.Settings, &event)
	if err != nil {
		ErrorLog.Println(err)
	}

	tos := []*sgmail.Email{}
	for _, toStr := range toStringArr {
		tos = append(tos, &sgmail.Email{Address: toStr})
	}

	emailHeaderInfo := sgEmailFields{
		Subject: fmt.Sprintf("OnePoint Active Directory User Provisioning: %s %s", employeeName, subjectType),
		From:    &sgmail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      tos,
	}

	err = sendTemplatedEmailSendGrid(emailHeaderInfo, AD_HF_SUMMARY_TEMPLATE, emailBody, "active_directory_user_provisioning_summary")
	if err != nil {
		ErrorLog.Println("sendTemplatedEmailSendGrid err: " + err.Error())
		return err
	}

	return nil
}

type HFCxnAlertEmailBody struct {
	ServiceName string
	Explanation template.HTML
}

func sendActiveDirectoryDisconnectedEmail(intURL, intName string, coInt CompanyIntegration) error {
	emailStruct, found := coInt.Settings["cxn_email_recipient"].(map[string]interface{})
	if !found {
		ErrorLog.Println("sendActiveDirectoryDisconnectedEmail no email address setting found")
		return errors.New("no email address setting!")
	}

	if emailStruct["address"] == "" {
		ErrorLog.Println("sendActiveDirectoryDisconnectedEmail email address setting was blank")
		return errors.New("email address setting was blank!")
	}

	toEmailAddress, found := emailStruct["address"].(string)
	if !found {
		ErrorLog.Println("sendActiveDirectoryDisconnectedEmail no email address setting found")
		return errors.New("no email address setting!")
	}

	url := fmt.Sprintf("https://connect.onehcm.com/integrations/%s", intURL)

	defaultExplanationFmt := "This could have occured due to a number of reasons including changed password, changed privileges, or an authentication timeout. Visit <a href=\"%s\">OnePoint Connect</a> to reauthenticate as soon as possible."
	if intURL == adIntegrationURL {
		defaultExplanationFmt = "Your On-Premise Service has most likely stopped running. Restart the service then check <a href=\"%s\">OnePoint Connect</a> to confirm your connection is back up."
	}

	explanationText := fmt.Sprintf(defaultExplanationFmt, url)

	emailBody := HFCxnAlertEmailBody{
		ServiceName: intName,
		Explanation: template.HTML(explanationText),
	}

	emailHeaderInfo := sgEmailFields{
		Subject: fmt.Sprintf("Your OnePoint/%s connection is DOWN", intName),
		From:    &sgmail.Email{Name: "OnePoint Connect", Address: passwords.NO_REPLY_EMAILER_ADDRESS},
		To:      []*sgmail.Email{&sgmail.Email{Address: toEmailAddress}},
	}

	err := sendTemplatedEmailSendGrid(emailHeaderInfo, SERVICE_DISCONNECTION_ALERT_TEMPLATE, emailBody, "user_provisioning_disconnection_alerts")
	if err != nil {
		ErrorLog.Println("sendTemplatedEmailSendGrid err: " + err.Error())
		return err
	}

	return nil
}
